package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/clover-eric/ato-cfip/internal/config"
	"github.com/clover-eric/ato-cfip/internal/publisher"
	"github.com/clover-eric/ato-cfip/internal/scheduler"
	"github.com/clover-eric/ato-cfip/internal/store"
	"github.com/clover-eric/ato-cfip/internal/web"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	once := flag.Bool("once", false, "run one cycle and exit")
	initConfig := flag.Bool("init", false, "run first-time config wizard")
	flag.Parse()

	if *initConfig {
		if err := config.InitWizard(*configPath); err != nil {
			log.Fatalf("init config: %v", err)
		}
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	st, err := store.Open(cfg.Storage.HistoryPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()
	pub, err := publisher.New(cfg.Publish)
	if err != nil {
		log.Fatalf("create publisher: %v", err)
	}
	runner := scheduler.NewRunner(cfg, st, pub)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *once {
		if err := runner.RunOnce(ctx); err != nil {
			log.Fatalf("run once: %v", err)
		}
		return
	}

	loc, err := time.LoadLocation(cfg.Server.Timezone)
	if err != nil {
		log.Printf("load timezone %q failed, using local timezone: %v", cfg.Server.Timezone, err)
		loc = time.Local
	}
	log.Printf("cfst-daemon started, schedule=%q timezone=%s", cfg.Server.Schedule, loc.String())
	var webServer *web.Server
	if cfg.Web.Enabled {
		webServer = web.New(cfg, runner)
		go func() {
			if err := webServer.Start(); err != nil && err != http.ErrServerClosed {
				log.Printf("web dashboard failed: %v", err)
				stop()
			}
		}()
	}
	initialized := web.IsInitialized()
	if !initialized {
		log.Printf("first-time setup is required; open the dashboard to initialize")
	}
	if cfg.Server.RunOnStart && initialized {
		go func() {
			if err := runner.RunOnce(ctx); err != nil {
				log.Printf("startup cycle failed: %v", err)
			}
		}()
	}
	if initialized {
		go scheduleLoop(ctx, cfg.Server.Schedule, loc, func() {
			if err := runner.RunOnce(ctx); err != nil {
				log.Printf("scheduled cycle failed: %v", err)
			}
		})
	}
	<-ctx.Done()
	log.Printf("shutting down")
	if webServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := webServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("web shutdown failed: %v", err)
		}
	}
}

func scheduleLoop(ctx context.Context, schedule string, loc *time.Location, task func()) {
	for {
		next := nextRun(time.Now().In(loc), schedule)
		timer := time.NewTimer(time.Until(next))
		log.Printf("next scheduled cycle at %s", next.Format(time.RFC3339))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			go task()
		}
	}
}

func nextRun(now time.Time, schedule string) time.Time {
	schedule = strings.TrimSpace(schedule)
	if strings.HasPrefix(schedule, "@every ") {
		if d, err := time.ParseDuration(strings.TrimSpace(strings.TrimPrefix(schedule, "@every "))); err == nil && d > 0 {
			return now.Add(d)
		}
	}
	// The default NAS-friendly schedule is hourly. "0 * * * *" means the next hour boundary.
	return now.Truncate(time.Hour).Add(time.Hour)
}
