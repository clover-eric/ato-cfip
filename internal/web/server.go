package web

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/clover-eric/ato-cfip/internal/config"
	"github.com/clover-eric/ato-cfip/internal/scheduler"
)

const runtimePath = "data/runtime.json"

type Server struct {
	cfg       config.Config
	runner    *scheduler.Runner
	server    *http.Server
	publicTpl *template.Template
	setupTpl  *template.Template
	adminTpl  *template.Template
	runtime   config.RuntimeConfig
}

type pageData struct {
	Title  string
	Domain string
}

func New(cfg config.Config, runner *scheduler.Runner) *Server {
	mux := http.NewServeMux()
	s := &Server{
		cfg:       cfg,
		runner:    runner,
		publicTpl: template.Must(template.New("public").Parse(publicHTML)),
		setupTpl:  template.Must(template.New("setup").Parse(setupHTML)),
		adminTpl:  template.Must(template.New("admin").Parse(adminHTML)),
		runtime:   loadRuntime(),
	}
	if s.runtime.Domain != "" {
		s.runner.SetDomain(s.runtime.Domain)
	}
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/admin", s.handleAdmin)
	mux.HandleFunc("/json", s.handlePublicStatus)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/public", s.handlePublicStatus)
	mux.HandleFunc("/api/run", s.handleRun)
	mux.HandleFunc("/api/setup", s.handleSetup)
	mux.HandleFunc("/api/login", s.handleLogin)
	mux.HandleFunc("/api/logout", s.handleLogout)
	mux.HandleFunc("/api/account", s.handleAccount)
	mux.HandleFunc("/api/config/domain", s.handleDomain)
	mux.HandleFunc("/healthz", s.handleHealth)
	s.server = &http.Server{
		Addr:              cfg.Web.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

func IsInitialized() bool {
	return loadRuntime().Initialized
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
	if !s.runtime.Initialized {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = s.setupTpl.Execute(w, pageData{Title: s.cfg.Web.Title, Domain: s.cfg.Publish.Domain})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.publicTpl.Execute(w, pageData{Title: s.cfg.Web.Title, Domain: s.cfg.Publish.Domain})
}

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/admin" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.adminTpl.Execute(w, pageData{Title: s.cfg.Web.Title, Domain: s.cfg.Publish.Domain})
}

func (s *Server) handlePublicStatus(w http.ResponseWriter, r *http.Request) {
	status := s.runner.Status()
	cfg := s.runner.Config()
	stats, _ := s.runner.ArchiveStats()
	published := len(status.Published.IPs)
	lastSpeed := 0.0
	if published > 0 {
		lastSpeed = status.Published.IPs[0].DownloadMBps
	}
	writeJSON(w, map[string]any{
		"setup": map[string]any{
			"initialized": s.runtime.Initialized,
		},
		"status": map[string]any{
			"running":       status.Running,
			"last_started":  status.LastStarted,
			"last_ended":    status.LastEnded,
			"last_error":    status.LastError,
			"stage":         status.Stage,
			"elapsed_sec":   status.ElapsedSec,
			"published_ips": published,
			"target":        status.Target,
			"top_speed":     lastSpeed,
			"progress":      status.Progress,
		},
		"archive": stats,
		"config": map[string]any{
			"domain":          cfg.Publish.Domain,
			"schedule":        cfg.Server.Schedule,
			"rounds_per_hour": cfg.Test.RoundsPerHour,
		},
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	status := s.runner.Status()
	cfg := s.runner.Config()
	stats, _ := s.runner.ArchiveStats()
	writeJSON(w, map[string]any{
		"setup": map[string]any{
			"initialized": s.runtime.Initialized,
			"admin_user":  s.runtime.AdminUser,
			"panel_url":   s.runtime.PanelURL,
		},
		"status":  status,
		"archive": stats,
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
	if !s.requireAuth(w, r) {
		return
	}
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

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.runtime.Initialized {
		http.Error(w, "setup already completed", http.StatusConflict)
		return
	}
	var req struct {
		AdminUser string `json:"admin_user"`
		Password  string `json:"password"`
		PanelURL  string `json:"panel_url"`
		Domain    string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	user := strings.TrimSpace(req.AdminUser)
	if user == "" {
		user = "admin"
	}
	if len(req.Password) < 8 {
		http.Error(w, "password must be at least 8 characters", http.StatusBadRequest)
		return
	}
	domain := strings.TrimSpace(req.Domain)
	if domain != "" {
		var err error
		domain, _, err = normalizeDomain(domain)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if domain == "" {
		domain = s.cfg.Publish.Domain
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	secret, err := randomToken()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.runtime = config.RuntimeConfig{
		Initialized:   true,
		AdminUser:     user,
		PasswordHash:  hash,
		SessionSecret: secret,
		PanelURL:      strings.TrimSpace(req.PanelURL),
		Domain:        domain,
	}
	s.runner.SetDomain(domain)
	if err := saveRuntime(s.runtime); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.setSessionCookie(w, user)
	go func() {
		if err := s.runner.RunOnce(context.Background()); err != nil {
			log.Printf("initial cycle failed: %v", err)
		}
	}()
	writeJSON(w, map[string]string{"status": "ok", "domain": domain})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.runtime.Initialized {
		http.Error(w, "setup required", http.StatusForbidden)
		return
	}
	var req struct {
		AdminUser string `json:"admin_user"`
		Password  string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if req.AdminUser != s.runtime.AdminUser || !verifyPassword(req.Password, s.runtime.PasswordHash) {
		http.Error(w, "invalid username or password", http.StatusUnauthorized)
		return
	}
	s.setSessionCookie(w, s.runtime.AdminUser)
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "ato_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleAccount(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		AdminUser       string `json:"admin_user"`
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if !verifyPassword(req.CurrentPassword, s.runtime.PasswordHash) {
		http.Error(w, "current password is incorrect", http.StatusUnauthorized)
		return
	}
	user := strings.TrimSpace(req.AdminUser)
	if user == "" {
		user = s.runtime.AdminUser
	}
	s.runtime.AdminUser = user
	if req.NewPassword != "" {
		if len(req.NewPassword) < 8 {
			http.Error(w, "new password must be at least 8 characters", http.StatusBadRequest)
			return
		}
		hash, err := hashPassword(req.NewPassword)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.runtime.PasswordHash = hash
		if secret, err := randomToken(); err == nil {
			s.runtime.SessionSecret = secret
		}
	}
	if err := saveRuntime(s.runtime); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.setSessionCookie(w, s.runtime.AdminUser)
	writeJSON(w, map[string]string{"status": "saved", "admin_user": s.runtime.AdminUser})
}

func (s *Server) handleDomain(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	domain, note, err := normalizeDomain(req.Domain)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.runner.SetDomain(domain)
	s.runtime.Domain = domain
	if err := saveRuntime(s.runtime); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{
		"status": "saved",
		"domain": domain,
		"note":   note,
	})
}

func (s *Server) requireSetup(w http.ResponseWriter) bool {
	if s.runtime.Initialized {
		return true
	}
	http.Error(w, "setup required", http.StatusForbidden)
	return false
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if !s.runtime.Initialized {
		http.Error(w, "setup required", http.StatusForbidden)
		return false
	}
	cookie, err := r.Cookie("ato_session")
	if err != nil || !s.validSession(cookie.Value) {
		http.Error(w, "login required", http.StatusUnauthorized)
		return false
	}
	return true
}

func loadRuntime() config.RuntimeConfig {
	b, err := os.ReadFile(runtimePath)
	if err != nil {
		return config.RuntimeConfig{}
	}
	var runtime config.RuntimeConfig
	if err := json.Unmarshal(b, &runtime); err != nil {
		return config.RuntimeConfig{}
	}
	return runtime
}

func saveRuntime(runtime config.RuntimeConfig) error {
	if err := os.MkdirAll(filepath.Dir(runtimePath), 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	b, err := json.MarshalIndent(runtime, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(runtimePath, b, 0o600)
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

func normalizeDomain(input string) (string, string, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return "", "", fmt.Errorf("domain is required")
	}
	note := ""
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return "", "", fmt.Errorf("invalid domain or url")
		}
		raw = u.Host
		note = "URL detected; saved host only because DNS records cannot include protocol or path."
	}
	host := raw
	if h, _, err := net.SplitHostPort(raw); err == nil {
		host = h
		note = "Port detected; saved host only because DNS records cannot include ports."
	} else if strings.Count(raw, ":") == 1 && !strings.Contains(raw, "]") {
		parts := strings.Split(raw, ":")
		if parts[0] != "" && parts[1] != "" {
			host = parts[0]
			note = "Port detected; saved host only because DNS records cannot include ports."
		}
	}
	host = strings.Trim(strings.ToLower(host), ".[] ")
	if host == "" || strings.ContainsAny(host, "/?#@") {
		return "", "", fmt.Errorf("invalid DNS host")
	}
	return host, note, nil
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}
	key := pbkdf2SHA256([]byte(password), salt, 100000, 32)
	return fmt.Sprintf("pbkdf2_sha256$100000$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func verifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2_sha256" {
		return false
	}
	iter := 100000
	if parts[1] != "100000" {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got := pbkdf2SHA256([]byte(password), salt, iter, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *Server) setSessionCookie(w http.ResponseWriter, user string) {
	expires := time.Now().Add(30 * 24 * time.Hour)
	payload := fmt.Sprintf("%s:%d", user, expires.Unix())
	sig := sign(payload, s.sessionSecret())
	http.SetCookie(w, &http.Cookie{
		Name:     "ato_session",
		Value:    payload + "." + sig,
		Path:     "/",
		Expires:  expires,
		MaxAge:   30 * 24 * 60 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) validSession(value string) bool {
	payload, sig, ok := strings.Cut(value, ".")
	if !ok || sign(payload, s.sessionSecret()) != sig {
		return false
	}
	user, expText, ok := strings.Cut(payload, ":")
	if !ok || user != s.runtime.AdminUser {
		return false
	}
	exp, err := strconv.ParseInt(expText, 10, 64)
	return err == nil && time.Now().Unix() < exp
}

func (s *Server) sessionSecret() string {
	if s.runtime.SessionSecret != "" {
		return s.runtime.SessionSecret
	}
	return s.runtime.PasswordHash
}

func sign(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func pbkdf2SHA256(password, salt []byte, iter, keyLen int) []byte {
	var out []byte
	var blockNum uint32 = 1
	for len(out) < keyLen {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		mac.Write([]byte{byte(blockNum >> 24), byte(blockNum >> 16), byte(blockNum >> 8), byte(blockNum)})
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iter; i++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
		blockNum++
	}
	return out[:keyLen]
}
