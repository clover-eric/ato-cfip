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
	"github.com/clover-eric/ato-cfip/internal/publisher"
	"github.com/clover-eric/ato-cfip/internal/scheduler"
	"github.com/clover-eric/ato-cfip/internal/subscription"
)

const runtimePath = "data/runtime.json"
const subscriptionsPath = "data/subscriptions.json"

type Server struct {
	cfg       config.Config
	runner    *scheduler.Runner
	server    *http.Server
	publicTpl *template.Template
	setupTpl  *template.Template
	adminTpl  *template.Template
	runtime   config.RuntimeConfig
	subStore  *subscription.Store
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
		publicTpl: template.Must(template.New("public").Parse(polishPublicHTML(publicHTML))),
		setupTpl:  template.Must(template.New("setup").Parse(setupHTML)),
		adminTpl:  template.Must(template.New("admin").Parse(polishAdminHTML(adminHTML))),
		runtime:   loadRuntime(),
		subStore:  subscription.NewStore(subscriptionsPath),
	}
	if s.runtime.Domain != "" {
		s.runner.SetDomain(s.runtime.Domain)
	}
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/setup", s.handleSetupPage)
	mux.HandleFunc("/admin", s.handleAdmin)
	mux.HandleFunc("/best.csv", s.handleBestCSV)
	mux.HandleFunc("/ips.csv", s.handleBestCSV)
	mux.HandleFunc("/json", s.handlePublicStatus)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/public", s.handlePublicStatus)
	mux.HandleFunc("/api/run", s.handleRun)
	mux.HandleFunc("/api/setup", s.handleSetup)
	mux.HandleFunc("/api/login", s.handleLogin)
	mux.HandleFunc("/api/logout", s.handleLogout)
	mux.HandleFunc("/api/account", s.handleAccount)
	mux.HandleFunc("/api/subscriptions", s.handleSubscriptions)
	mux.HandleFunc("/api/subscriptions/", s.handleSubscriptionItem)
	mux.HandleFunc("/api/config/domain", s.handleDomain)
	mux.HandleFunc("/api/cloudflare/bind", s.handleCloudflareBind)
	mux.HandleFunc("/sub/", s.handlePublicSubscription)
	mux.HandleFunc("/healthz", s.handleHealth)
	s.server = &http.Server{
		Addr:              cfg.Web.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

func polishPublicHTML(html string) string {
	return strings.ReplaceAll(
		html,
		`&#27492;&#39029;&#21482;&#23637;&#31034;&#31995;&#32479;&#29366;&#24577;&#65292;&#19981;&#20844;&#24320;&#20248;&#36873; IP&#12290;`,
		`&#27492;&#39029;&#21482;&#23637;&#31034;&#31995;&#32479;&#29366;&#24577;&#65292;&#19981;&#30452;&#25509;&#23637;&#31034;&#20248;&#36873; IP&#12290;CSV &#32467;&#26524;&#28304;&#65306;/best.csv`,
	)
}

func polishAdminHTML(html string) string {
	html = strings.ReplaceAll(
		html,
		`<button id="domainBtn">&#32465;&#23450;&#22495;&#21517;</button><button id="accountBtn">`,
		`<button id="domainBtn">&#32465;&#23450;&#22495;&#21517;</button><button id="subBtn">&#35746;&#38405;</button><button id="accountBtn">`,
	)
	html = strings.ReplaceAll(
		html,
		`<div class="modal" id="accountModal" hidden>`,
		`<div class="modal" id="subModal" hidden><div class="box"><h2>&#29983;&#25104;&#20248;&#36873;&#35746;&#38405;&#38142;&#25509;</h2><div class="guide"><b>&#19968;&#38190;&#36866;&#37197;&#20248;&#36873;&#22495;&#21517;</b><br>&#31896;&#36148;&#20320;&#30340;&#26426;&#22330;&#35746;&#38405;&#38142;&#25509;&#12289;&#33258;&#24314;&#35746;&#38405;&#38142;&#25509;&#65292;&#25110;&#32773; VLESS / VMess / Trojan / SS &#31561;&#33410;&#28857;&#38142;&#25509;&#12290;&#31995;&#32479;&#20250;&#20445;&#30041;&#21407;&#33410;&#28857;&#30340; SNI&#12289;Host&#12289;&#36335;&#24452;&#12289;&#23494;&#38053;&#21644;&#31471;&#21475;&#65292;&#21482;&#25226;&#36830;&#25509;&#22320;&#22336;&#25442;&#25104;&#24403;&#21069;&#20248;&#36873;&#22495;&#21517;&#12290;</div><div class="form"><input id="subName" placeholder="&#21517;&#31216;&#65292;&#21487;&#36873;"><textarea id="subSource" placeholder="&#31896;&#36148;&#35746;&#38405;&#38142;&#25509;&#25110;&#33410;&#28857;&#38142;&#25509;&#65292;&#27599;&#34892;&#19968;&#20010;" style="min-height:160px;border:1px solid var(--line);border-radius:8px;padding:11px 12px;font:inherit;width:100%;resize:vertical"></textarea><div class="err" id="se"></div><button class="primary" id="genSub">&#19968;&#38190;&#29983;&#25104;</button><button id="closeSub">&#21462;&#28040;</button></div><div class="guide" id="subOut" hidden></div><div class="guide" id="subList"></div></div></div><div class="modal" id="accountModal" hidden>`,
	)
	html = strings.ReplaceAll(
		html,
		`domainBtn.onclick=()=>{domainModal.hidden=false;setStep(1)};accountBtn.onclick=()=>{`,
		`domainBtn.onclick=()=>{domainModal.hidden=false;setStep(1)};subBtn.onclick=()=>{subModal.hidden=false;subOut.hidden=true;loadSubs()};closeSub.onclick=()=>subModal.hidden=true;genSub.onclick=genSubscription;accountBtn.onclick=()=>{`,
	)
	html = strings.ReplaceAll(
		html,
		`function renderTable(ips){`,
		`async function genSubscription(){se.textContent='';genSub.disabled=true;genSub.textContent='生成中...';try{const d=await api('/api/subscriptions',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:subName.value,source:subSource.value})});subOut.hidden=false;subOut.innerHTML='<b>生成成功</b><br><input id="subUrl" readonly value="'+esc(d.url)+'"><div class="actions"><button class="primary" id="copySub">复制链接</button><a class="btn" target="_blank" href="'+esc(d.url)+'">打开订阅</a></div><div class="sub">已改写 '+Number(d.converted||0)+' 个节点，输出格式 '+esc(String(d.encoding||''))+'</div>';copySub.onclick=()=>copyText(d.url);subSource.value='';loadSubs()}catch(e){se.textContent=e.message}finally{genSub.disabled=false;genSub.textContent='一键生成'}}async function loadSubs(){try{const d=await api('/api/subscriptions',{cache:'no-store'}),items=d.items||[];if(!items.length){subList.innerHTML='<b>已生成订阅</b><br><span class="sub">暂无订阅链接</span>';return}subList.innerHTML='<b>已生成订阅</b>'+items.map(x=>'<div class="row"><span>'+esc(x.name||'ATO CFIP')+'<br><span class="sub">'+esc(x.source_type||'raw')+'</span></span><span><input readonly value="'+esc(x.url)+'"><div class="actions"><button data-copy="'+esc(x.url)+'">复制</button><button data-del="'+esc(x.token)+'">删除</button></div></span></div>').join('');subList.querySelectorAll('[data-copy]').forEach(b=>b.onclick=()=>copyText(b.dataset.copy));subList.querySelectorAll('[data-del]').forEach(b=>b.onclick=()=>delSub(b.dataset.del))}catch(e){subList.innerHTML='<span class="err">'+esc(e.message)+'</span>'}}async function delSub(t){if(!confirm('确认删除这个订阅链接？'))return;await api('/api/subscriptions/'+encodeURIComponent(t),{method:'DELETE'});loadSubs()}function copyText(v){if(navigator.clipboard)navigator.clipboard.writeText(v)}function renderTable(ips){`,
	)
	html = strings.ReplaceAll(
		html,
		`['\u6bcf\u5c0f\u65f6\u8f6e\u6b21',c.rounds_per_hour],['\u4e0b\u8f7d\u5730\u5740',c.download_url],['\u76d1\u542c\u5730\u5740',c.web_listen]]`,
		`['\u6bcf\u5c0f\u65f6\u8f6e\u6b21',c.rounds_per_hour],['\u6700\u4f4e\u53d1\u5e03\u901f\u5ea6',Number(c.min_speed_mb||0).toFixed(1)+' MB/s'],['\u6700\u5927\u53d1\u5e03\u5ef6\u8fdf',Number(c.max_delay_ms||0)+' ms'],['\u4e0b\u8f7d\u5019\u9009\u6570',c.download_candidates],['\u4e0b\u8f7d\u7ebf\u7a0b',c.download_threads],['\u4e0b\u8f7d\u5730\u5740',c.download_url],['\u76d1\u542c\u5730\u5740',c.web_listen]]`,
	)
	html = strings.ReplaceAll(
		html,
		`\u6682\u65e0\u53d1\u5e03\u7ed3\u679c\uff0c\u6216\u672c\u8f6e\u672a\u6d4b\u5230\u6709\u6548\u4e0b\u8f7d\u901f\u5ea6`,
		`\u6682\u65e0\u53d1\u5e03\u7ed3\u679c\uff0c\u6216\u672c\u8f6e\u672a\u6d4b\u5230\u8fbe\u6807\u901f\u5ea6\u7684 IP`,
	)
	html = strings.ReplaceAll(
		html,
		`<td>'+x.round+'</td>`,
		`<td>'+(x.round===0?'\u6c60':x.round)+'</td>`,
	)
	html = strings.ReplaceAll(
		html,
		`.replace(/Selected (\d+)\/(\d+) IPs/,'\u5df2\u9009\u51fa $1/$2 \u4e2a IP')`,
		`.replace(/Selected (\d+)\/(\d+) IPs above ([0-9.]+) MB\/s/,'\u5df2\u9009\u51fa $1/$2 \u4e2a\u8fbe\u6807 IP\uff08\u2265 $3 MB/s\uff09').replace(/Selected (\d+)\/(\d+) IPs/,'\u5df2\u9009\u51fa $1/$2 \u4e2a IP')`,
	)
	html = strings.ReplaceAll(
		html,
		`.replace(/Round (\d+) produced no positive-speed result/,'\u7b2c $1 \u8f6e\u672a\u6d4b\u5230\u6709\u6548\u901f\u5ea6')`,
		`.replace(/Round (\d+) produced no IP above ([0-9.]+) MB\/s/,'\u7b2c $1 \u8f6e\u6ca1\u6709\u8fbe\u5230 $2 MB/s \u7684 IP').replace(/Round (\d+) produced no positive-speed result/,'\u7b2c $1 \u8f6e\u672a\u6d4b\u5230\u6709\u6548\u901f\u5ea6')`,
	)
	html = strings.ReplaceAll(
		html,
		`return v.replace('Finished','\u8fd0\u884c\u5b8c\u6210')`,
		`return v.replace('Finished','\u8fd0\u884c\u5b8c\u6210').replace(/Rechecking stable IP pool (\d+)\/(\d+)/,'\u590d\u6d4b\u7a33\u5b9a IP \u6c60 $1/$2').replace(/Pool recheck kept (\d+)\/(\d+) IPs, (\d+) need replacement/,'IP \u6c60\u590d\u6d4b\u4fdd\u7559 $1/$2\uff0c\u9700\u66ff\u6362 $3 \u4e2a').replace(/Scanning public IP library, need (\d+) replacements/,'\u6b63\u5728\u4ece\u516c\u5171 IP \u5e93\u8865\u5145 $1 \u4e2a\u66ff\u6362 IP').replace(/qualified IP pool incomplete: (\d+)\/(\d+) IPs meet speed >= ([0-9.]+) MB\/s and delay <= (\d+) ms; keeping previous published pool/,'\u8fbe\u6807 IP \u6c60\u672a\u51d1\u6ee1\uff1a$1/$2\uff08\u2265 $3 MB/s\uff0c\u2264 $4 ms\uff09\uff0c\u5df2\u4fdd\u7559\u4e0a\u4e00\u7248\u53d1\u5e03\u7ed3\u679c')`,
	)
	html = strings.ReplaceAll(
		html,
		`<b>自动绑定优选域名</b><br>输入你想使用的优选域名，例如 cf.example.com。系统会自动识别 Cloudflare Zone，并创建或更新 DNS 记录。`,
		`<b>绑定到面板程序</b><br>输入你想使用的域名，例如 cf.example.com。系统会把该域名指向当前 NAS 面板入口；优选结果保存在本机 CSV，不再写入 Cloudflare 优选 IP 记录。目标地址留空时会自动检测公网 IP。`,
	)
	html = strings.ReplaceAll(
		html,
		`<input id="bindDomain" placeholder="cf.example.com"><input id="bindToken" type="password" placeholder="粘贴 Cloudflare API Token">`,
		`<input id="bindDomain" placeholder="cf.example.com"><input id="bindTarget" placeholder="面板目标地址，可留空自动检测公网 IP"><input id="bindToken" type="password" placeholder="粘贴 Cloudflare API Token">`,
	)
	html = strings.ReplaceAll(
		html,
		`JSON.stringify({domain:bindDomain.value,api_token:bindToken.value,proxied:false})`,
		`JSON.stringify({domain:bindDomain.value,target:bindTarget.value,api_token:bindToken.value,proxied:false})`,
	)
	html = strings.ReplaceAll(
		html,
		`<input id="bindDomain" placeholder="cf.example.com"><input id="bindTarget"`,
		`<input id="bindDomain" placeholder="cf.example.com"><input id="bindPanelURL" placeholder="面板访问地址，留空默认 https://绑定域名"><input id="bindTarget"`,
	)
	html = strings.ReplaceAll(
		html,
		`try{const d=await api('/api/cloudflare/bind'`,
		`try{if(!bindPanelURL.value&&bindDomain.value)bindPanelURL.value='https://'+bindDomain.value.trim();const d=await api('/api/cloudflare/bind'`,
	)
	html = strings.ReplaceAll(
		html,
		`JSON.stringify({domain:bindDomain.value,target:bindTarget.value,api_token:bindToken.value,proxied:false})`,
		`JSON.stringify({domain:bindDomain.value,panel_url:bindPanelURL.value,target:bindTarget.value,api_token:bindToken.value,proxied:false})`,
	)
	html = strings.ReplaceAll(
		html,
		`'+d.csv_url;`,
		`'+d.csv_url+'；订阅链接将使用 '+d.panel_url;`,
	)
	html = strings.ReplaceAll(
		html,
		`bindDone.textContent='域名 '+d.domain+' 已绑定到 Cloudflare Zone '+d.zone_name+'，已发布 IP 数量：'+d.published;`,
		`bindDone.textContent='域名 '+d.domain+' 已指向 '+d.record_type+' '+d.target+'；CSV 结果源：'+d.csv_url;`,
	)
	html = strings.ReplaceAll(
		html,
		`'+d.csv_url;`,
		`'+d.csv_url+'；订阅链接将使用 '+d.panel_url;`,
	)
	html = strings.ReplaceAll(
		html,
		`['\u76d1\u542c\u5730\u5740',c.web_listen]]`,
		`['\u76d1\u542c\u5730\u5740',c.web_listen],['CSV \u7ed3\u679c\u6e90',c.csv_url||'/best.csv']]`,
	)
	html = strings.ReplaceAll(
		html,
		`['CSV \u7ed3\u679c\u6e90',c.csv_url||'/best.csv']]`,
		`['\u9762\u677f\u8bbf\u95ee\u5730\u5740',c.panel_url||''],['CSV \u7ed3\u679c\u6e90',c.csv_url||'/best.csv']]`,
	)
	return html
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
		s.renderSetup(w)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.publicTpl.Execute(w, pageData{Title: s.cfg.Web.Title, Domain: s.cfg.Publish.Domain})
}

func (s *Server) handleSetupPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/setup" {
		http.NotFound(w, r)
		return
	}
	if s.runtime.Initialized {
		http.Redirect(w, r, "/admin", http.StatusFound)
		return
	}
	s.renderSetup(w)
}

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/admin" {
		http.NotFound(w, r)
		return
	}
	if !s.runtime.Initialized {
		http.Redirect(w, r, "/setup", http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.adminTpl.Execute(w, pageData{Title: s.cfg.Web.Title, Domain: s.cfg.Publish.Domain})
}

func (s *Server) renderSetup(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.setupTpl.Execute(w, pageData{Title: s.cfg.Web.Title, Domain: s.cfg.Publish.Domain})
}

func (s *Server) handleBestCSV(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/best.csv" && r.URL.Path != "/ips.csv" {
		http.NotFound(w, r)
		return
	}
	if !s.runtime.Initialized {
		http.Error(w, "setup required", http.StatusServiceUnavailable)
		return
	}
	status := s.runner.Status()
	if len(status.Published.IPs) == 0 {
		http.Error(w, "preferred ip pool is not ready", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", `inline; filename="best_ips.csv"`)
	_, _ = w.Write(publisher.FormatCSV(status.Published))
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
			"domain":            cfg.Publish.Domain,
			"domain_configured": isConfiguredDomain(cfg.Publish.Domain),
			"schedule":          cfg.Server.Schedule,
			"rounds_per_hour":   cfg.Test.RoundsPerHour,
			"panel_url":         s.publicBaseURL(r),
			"csv_url":           s.publicURL(r, "/best.csv"),
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
			"domain":              cfg.Publish.Domain,
			"publish_mode":        cfg.Publish.Mode,
			"schedule":            cfg.Server.Schedule,
			"timezone":            cfg.Server.Timezone,
			"rounds_per_hour":     cfg.Test.RoundsPerHour,
			"desired_unique_ips":  cfg.Test.DesiredUniqueIPs,
			"download_time":       cfg.Test.DownloadTimeSeconds,
			"download_candidates": cfg.Test.DownloadCandidates,
			"download_threads":    cfg.Test.DownloadThreads,
			"min_speed_mb":        cfg.Test.MinSpeedMB,
			"max_delay_ms":        cfg.Test.MaxDelayMS,
			"download_url":        cfg.Test.URL,
			"web_listen":          cfg.Web.Listen,
			"domain_configured":   isConfiguredDomain(cfg.Publish.Domain),
			"csv_file":            cfg.Publish.CSVFile,
			"panel_url":           s.publicBaseURL(r),
			"csv_url":             s.publicURL(r, "/best.csv"),
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

func (s *Server) handleSubscriptions(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		entries, err := s.subStore.List()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items := make([]map[string]any, 0, len(entries))
		for _, entry := range entries {
			items = append(items, s.subscriptionSummary(r, entry))
		}
		writeJSON(w, map[string]any{"items": items})
	case http.MethodPost:
		var req struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json body", http.StatusBadRequest)
			return
		}
		source := strings.TrimSpace(req.Source)
		if source == "" {
			http.Error(w, "subscription source is required", http.StatusBadRequest)
			return
		}
		cfg := s.runner.Config()
		addresses, err := s.subscriptionAddresses(cfg)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		token, err := subscription.NewToken()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		now := time.Now()
		entry := subscription.Entry{
			Token:      token,
			Name:       strings.TrimSpace(req.Name),
			Source:     source,
			SourceType: subscription.DetectSourceType(source),
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
		defer cancel()
		result, err := subscription.RenderWithAddresses(ctx, entry, addresses)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if result.Converted == 0 {
			http.Error(w, "no supported proxy nodes were found in this subscription", http.StatusBadRequest)
			return
		}
		if err := s.subStore.Save(entry); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp := s.subscriptionSummary(r, entry)
		resp["converted"] = result.Converted
		resp["encoding"] = result.Encoding
		writeJSON(w, resp)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSubscriptionItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	token := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/subscriptions/"), "/")
	if token == "" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.subStore.Delete(token); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *Server) handlePublicSubscription(w http.ResponseWriter, r *http.Request) {
	token := strings.Trim(strings.TrimPrefix(r.URL.Path, "/sub/"), "/")
	if token == "" || strings.Contains(token, "/") {
		http.NotFound(w, r)
		return
	}
	entry, ok, err := s.subStore.Get(token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	cfg := s.runner.Config()
	addresses, err := s.subscriptionAddresses(cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	result, err := subscription.RenderWithAddresses(ctx, entry, addresses)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-ATO-CFIP-Converted", strconv.Itoa(result.Converted))
	w.Header().Set("X-ATO-CFIP-Encoding", result.Encoding)
	_, _ = io.WriteString(w, result.Content)
}

func (s *Server) subscriptionAddresses(cfg config.Config) ([]string, error) {
	status := s.runner.Status()
	addresses := make([]string, 0, len(status.Published.IPs))
	seen := make(map[string]struct{})
	for _, item := range status.Published.IPs {
		ip := strings.TrimSpace(item.IP)
		if ip == "" {
			continue
		}
		if _, exists := seen[ip]; exists {
			continue
		}
		seen[ip] = struct{}{}
		addresses = append(addresses, ip)
	}
	if len(addresses) > 0 {
		return addresses, nil
	}
	if isConfiguredDomain(cfg.Publish.Domain) {
		return []string{cfg.Publish.Domain}, nil
	}
	return nil, fmt.Errorf("preferred IP pool is not ready; run a speed test first")
}

func (s *Server) subscriptionSummary(r *http.Request, entry subscription.Entry) map[string]any {
	name := entry.Name
	if name == "" {
		name = "ATO CFIP"
	}
	return map[string]any{
		"token":       entry.Token,
		"name":        name,
		"source_type": entry.SourceType,
		"created_at":  entry.CreatedAt,
		"updated_at":  entry.UpdatedAt,
		"url":         s.publicURL(r, "/sub/"+entry.Token),
	}
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
		Domain   string `json:"domain"`
		PanelURL string `json:"panel_url"`
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
	panelURLRaw := strings.TrimSpace(req.PanelURL)
	if panelURLRaw == "" {
		panelURLRaw = "https://" + domain
	}
	panelURL, err := normalizePublicBaseURL(panelURLRaw, "https")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.runtime.PanelURL = panelURL
	if err := saveRuntime(s.runtime); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{
		"status":    "saved",
		"domain":    domain,
		"panel_url": panelURL,
		"note":      note,
	})
}

func (s *Server) handleCloudflareBind(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Domain   string `json:"domain"`
		APIToken string `json:"api_token"`
		PanelURL string `json:"panel_url"`
		Target   string `json:"target"`
		Proxied  bool   `json:"proxied"`
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
	token := strings.TrimSpace(req.APIToken)
	if token == "" {
		http.Error(w, "cloudflare api token is required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	zone, err := publisher.FindZone(ctx, token, domain)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	target, targetNote, err := detectPanelTarget(ctx, req.Target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	record, err := publisher.UpsertPanelRecord(ctx, token, zone.ID, domain, target, s.runner.Config().Publish.TTL, req.Proxied)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	panelURLRaw := strings.TrimSpace(req.PanelURL)
	if panelURLRaw == "" {
		panelURLRaw = "https://" + domain
	}
	panelURL, err := normalizePublicBaseURL(panelURLRaw, "https")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg := s.runner.Config().Publish
	cfg.Mode = "file"
	cfg.Domain = domain
	cfg.Cloudflare.APIToken = token
	cfg.Cloudflare.ZoneID = zone.ID
	cfg.Cloudflare.Proxied = req.Proxied
	pub, err := publisher.New(cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.runner.SetPublisher(cfg, pub)
	s.runtime.Domain = domain
	s.runtime.PanelURL = panelURL
	s.runtime.Cloudflare = config.CloudflareRuntimeConfig{
		APIToken: token,
		ZoneID:   zone.ID,
		ZoneName: zone.Name,
		Proxied:  req.Proxied,
	}
	if err := saveRuntime(s.runtime); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	status := s.runner.Status()
	if len(status.Published.IPs) > 0 {
		if err := pub.Publish(ctx, status.Published); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, map[string]any{
		"status":      "saved",
		"domain":      domain,
		"zone_id":     zone.ID,
		"zone_name":   zone.Name,
		"record_type": record.RecordType,
		"target":      record.Target,
		"proxied":     record.Proxied,
		"panel_url":   panelURL,
		"csv_url":     panelURL + "/best.csv",
		"sub_base":    panelURL + "/sub/",
		"published":   len(status.Published.IPs),
		"note":        strings.TrimSpace(note + " " + targetNote),
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

func requestBaseURL(r *http.Request) string {
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host
}

func (s *Server) publicURL(r *http.Request, path string) string {
	base := strings.TrimRight(s.publicBaseURL(r), "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

func (s *Server) publicBaseURL(r *http.Request) string {
	cfg := s.runner.Config()
	base := strings.TrimSpace(s.runtime.PanelURL)
	if base == "" && isConfiguredDomain(cfg.Publish.Domain) {
		base = "https://" + cfg.Publish.Domain
	}
	if base != "" {
		if normalized, err := normalizePublicBaseURL(base, requestScheme(r)); err == nil {
			return normalized
		}
	}
	return requestBaseURL(r)
}

func requestScheme(r *http.Request) string {
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme != "" {
		return scheme
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func normalizePublicBaseURL(input, fallbackScheme string) (string, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return "", fmt.Errorf("panel url is required")
	}
	if fallbackScheme == "" {
		fallbackScheme = "https"
	}
	if !strings.Contains(raw, "://") {
		raw = fallbackScheme + "://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid panel url")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("panel url must start with http:// or https://")
	}
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), nil
}

func detectPanelTarget(ctx context.Context, input string) (string, string, error) {
	target := strings.TrimSpace(input)
	if target != "" {
		normalized, err := normalizePanelTarget(target)
		if err != nil {
			return "", "", err
		}
		return normalized, "", nil
	}
	client := &http.Client{Timeout: 10 * time.Second}
	for _, endpoint := range []string{"https://api.ipify.org", "https://ifconfig.me/ip"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 128))
		_ = resp.Body.Close()
		if readErr != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			continue
		}
		ip := strings.TrimSpace(string(body))
		if parsed := net.ParseIP(ip); parsed != nil {
			return parsed.String(), "Auto-detected current public IP.", nil
		}
	}
	return "", "", fmt.Errorf("could not auto-detect public IP; please enter your NAS public IP or a CNAME target")
}

func normalizePanelTarget(input string) (string, error) {
	target := strings.TrimSpace(input)
	if target == "" {
		return "", fmt.Errorf("panel target is required")
	}
	if ip := net.ParseIP(target); ip != nil {
		return ip.String(), nil
	}
	if strings.Contains(target, "://") {
		u, err := url.Parse(target)
		if err != nil || u.Host == "" {
			return "", fmt.Errorf("invalid panel target")
		}
		target = u.Host
	}
	if h, _, err := net.SplitHostPort(target); err == nil {
		target = h
	}
	target = strings.Trim(strings.ToLower(target), ".[] ")
	if target == "" || strings.ContainsAny(target, "/?#@") {
		return "", fmt.Errorf("invalid panel target")
	}
	return target, nil
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

func isConfiguredDomain(domain string) bool {
	domain = strings.TrimSpace(strings.ToLower(domain))
	return domain != "" && domain != "not-configured.local" && domain != "best.example.com"
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
