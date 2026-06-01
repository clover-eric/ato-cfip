package web

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/clover-eric/ato-cfip/internal/config"
	"github.com/clover-eric/ato-cfip/internal/scheduler"
)

type Server struct {
	cfg    config.Config
	runner *scheduler.Runner
	server *http.Server
	tpl    *template.Template
}

type pageData struct {
	Title  string
	Domain string
}

func New(cfg config.Config, runner *scheduler.Runner) *Server {
	mux := http.NewServeMux()
	s := &Server{
		cfg:    cfg,
		runner: runner,
		tpl:    template.Must(template.New("dashboard").Parse(dashboardHTML)),
	}
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/run", s.handleRun)
	mux.HandleFunc("/healthz", s.handleHealth)
	s.server = &http.Server{
		Addr:              cfg.Web.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

func (s *Server) Start() error {
	log.Printf("web dashboard listening on %s", s.cfg.Web.Listen)
	return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.tpl.Execute(w, pageData{Title: s.cfg.Web.Title, Domain: s.cfg.Publish.Domain})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := s.runner.Status()
	cfg := s.runner.Config()
	writeJSON(w, map[string]any{
		"status": status,
		"config": map[string]any{
			"domain":             cfg.Publish.Domain,
			"publish_mode":       cfg.Publish.Mode,
			"schedule":           cfg.Server.Schedule,
			"timezone":           cfg.Server.Timezone,
			"rounds_per_hour":    cfg.Test.RoundsPerHour,
			"desired_unique_ips": cfg.Test.DesiredUniqueIPs,
			"download_time":      cfg.Test.DownloadTimeSeconds,
			"download_url":       cfg.Test.URL,
			"web_listen":         cfg.Web.Listen,
		},
	})
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	go func() {
		if err := s.runner.RunOnce(context.Background()); err != nil {
			log.Printf("manual cycle failed: %v", err)
		}
	}()
	writeJSON(w, map[string]string{"status": "started"})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	_, _ = fmt.Fprintln(w, "ok")
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
