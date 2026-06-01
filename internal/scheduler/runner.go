package scheduler

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/clover-eric/ato-cfip/internal/config"
	"github.com/clover-eric/ato-cfip/internal/engine"
	"github.com/clover-eric/ato-cfip/internal/model"
	"github.com/clover-eric/ato-cfip/internal/publisher"
	"github.com/clover-eric/ato-cfip/internal/store"
)

type Runner struct {
	cfg       config.Config
	store     *store.Store
	publisher publisher.Publisher
	mu        sync.Mutex
	running   bool
	lastRun   RunStatus
}

type RunStatus struct {
	Running     bool               `json:"running"`
	LastStarted *time.Time         `json:"last_started,omitempty"`
	LastEnded   *time.Time         `json:"last_ended,omitempty"`
	LastError   string             `json:"last_error,omitempty"`
	Published   model.PublishedSet `json:"published"`
	Rounds      int                `json:"rounds"`
	Target      int                `json:"target"`
}

func NewRunner(cfg config.Config, st *store.Store, pub publisher.Publisher) *Runner {
	r := &Runner{cfg: cfg, store: st, publisher: pub}
	if cycle, err := st.LastCycle(); err == nil && cycle != nil && len(cycle.Published) > 0 {
		r.lastRun.Published = model.PublishedSet{
			Domain:      cfg.Publish.Domain,
			GeneratedAt: cycle.CreatedAt,
			IPs:         cycle.Published,
		}
		r.lastRun.LastStarted = &cycle.StartedAt
		r.lastRun.LastEnded = &cycle.CreatedAt
	}
	return r
}

func (r *Runner) RunOnce(ctx context.Context) error {
	if !r.beginRun() {
		return fmt.Errorf("speed-test cycle is already running")
	}
	started := time.Now()
	r.setStarted(started)
	var runErr error
	defer func() {
		r.finishRun(runErr)
	}()
	log.Printf("starting speed-test cycle: rounds=%d target_unique=%d", r.cfg.Test.RoundsPerHour, r.cfg.Test.DesiredUniqueIPs)
	selected := make(map[string]model.Result)
	all := make([]model.Result, 0)
	totalRounds := r.cfg.Test.RoundsPerHour + r.cfg.Test.MaxExtraRounds
	if totalRounds < r.cfg.Test.RoundsPerHour {
		totalRounds = r.cfg.Test.RoundsPerHour
	}

	for round := 1; round <= totalRounds; round++ {
		if round > r.cfg.Test.RoundsPerHour && len(selected) >= r.cfg.Test.DesiredUniqueIPs {
			break
		}
		results, err := engine.New(r.cfg.Test).Run(ctx, round)
		if err != nil {
			log.Printf("round %d failed: %v", round, err)
			continue
		}
		if len(results) == 0 {
			log.Printf("round %d produced no usable result", round)
			continue
		}
		all = append(all, results...)
		winner := firstNewWinner(results, selected)
		if winner == nil {
			log.Printf("round %d winner duplicated, no new unique IP found in candidate list", round)
			continue
		}
		selected[winner.IP] = *winner
		log.Printf("round %d winner: %s %.2f MB/s %.2f ms", round, winner.IP, winner.DownloadMBps, winner.DelayMS)
	}

	final := make([]model.Result, 0, len(selected))
	for _, item := range selected {
		final = append(final, item)
	}
	sort.Slice(final, func(i, j int) bool {
		if final[i].DownloadMBps != final[j].DownloadMBps {
			return final[i].DownloadMBps > final[j].DownloadMBps
		}
		return final[i].DelayMS < final[j].DelayMS
	})
	if len(final) > r.cfg.Test.DesiredUniqueIPs {
		final = final[:r.cfg.Test.DesiredUniqueIPs]
	}
	if len(final) == 0 {
		runErr = fmt.Errorf("no publishable IPs found")
		return runErr
	}
	set := model.PublishedSet{
		Domain:      r.cfg.Publish.Domain,
		GeneratedAt: time.Now(),
		IPs:         final,
	}
	if err := r.store.SaveCycle(ctx, started, all, final); err != nil {
		runErr = err
		return runErr
	}
	if err := r.publisher.Publish(ctx, set); err != nil {
		runErr = err
		return runErr
	}
	r.setPublished(set)
	if r.cfg.Storage.KeepDays > 0 {
		if err := r.store.Prune(ctx, r.cfg.Storage.KeepDays); err != nil {
			log.Printf("prune old data failed: %v", err)
		}
	}
	log.Printf("cycle finished: published=%d elapsed=%s", len(final), time.Since(started).Round(time.Second))
	return nil
}

func (r *Runner) Status() RunStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	status := r.lastRun
	status.Running = r.running
	status.Rounds = r.cfg.Test.RoundsPerHour
	status.Target = r.cfg.Test.DesiredUniqueIPs
	return status
}

func (r *Runner) Config() config.Config {
	return r.cfg
}

func (r *Runner) beginRun() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return false
	}
	r.running = true
	return true
}

func (r *Runner) setStarted(t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastRun.Running = true
	r.lastRun.LastStarted = &t
	r.lastRun.LastError = ""
}

func (r *Runner) setPublished(set model.PublishedSet) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastRun.Published = set
}

func (r *Runner) finishRun(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = false
	r.lastRun.Running = false
	ended := time.Now()
	r.lastRun.LastEnded = &ended
	if err != nil {
		r.lastRun.LastError = err.Error()
	}
}

func firstNewWinner(results []model.Result, selected map[string]model.Result) *model.Result {
	for _, result := range results {
		if _, exists := selected[result.IP]; exists {
			continue
		}
		cp := result
		return &cp
	}
	return nil
}
