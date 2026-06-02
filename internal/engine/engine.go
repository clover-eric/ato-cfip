package engine

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/VividCortex/ewma"
	"github.com/clover-eric/ato-cfip/internal/config"
	"github.com/clover-eric/ato-cfip/internal/model"
)

const (
	tcpConnectTimeout = time.Second
	bufferSize        = 1024
)

var (
	regexpColoIATA    = regexp.MustCompile(`[A-Z]{3}`)
	regexpColoCountry = regexp.MustCompile(`[A-Z]{2}`)
	regexpColoGcore   = regexp.MustCompile(`^[a-z]{2}`)
)

type Engine struct {
	cfg        config.TestConfig
	onProgress func(Progress)
}

type Progress struct {
	Phase         string  `json:"phase"`
	DelayDone     int     `json:"delay_done"`
	DelayTotal    int     `json:"delay_total"`
	Available     int     `json:"available"`
	DownloadDone  int     `json:"download_done"`
	DownloadTotal int     `json:"download_total"`
	CurrentIP     string  `json:"current_ip,omitempty"`
	LastIP        string  `json:"last_ip,omitempty"`
	LastSpeedMBps float64 `json:"last_speed_mbps,omitempty"`
	UsableResults int     `json:"usable_results"`
}

type pingData struct {
	ip       *net.IPAddr
	sent     int
	received int
	delay    time.Duration
	colo     string
}

func New(cfg config.TestConfig) *Engine {
	return &Engine{cfg: normalizeConfig(cfg)}
}

func (e *Engine) WithProgress(fn func(Progress)) *Engine {
	e.onProgress = fn
	return e
}

func normalizeConfig(cfg config.TestConfig) config.TestConfig {
	if cfg.DelayThreads <= 0 {
		cfg.DelayThreads = 200
	}
	if cfg.PingTimes <= 0 {
		cfg.PingTimes = 4
	}
	if cfg.DownloadCandidates <= 0 {
		cfg.DownloadCandidates = 10
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		cfg.Port = 443
	}
	if cfg.MaxDelayMS <= 0 {
		cfg.MaxDelayMS = 9999
	}
	if cfg.URL == "" {
		cfg.URL = "https://speed.cloudflare.com/__down?bytes=50000000"
	}
	return cfg
}

func (e *Engine) Run(ctx context.Context, round int) ([]model.Result, error) {
	rand.Seed(time.Now().UnixNano())
	ips, err := loadIPRanges(e.cfg.IPFile, e.cfg.IPText, e.cfg.IPv6, e.cfg.AllIP)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no candidate IPs loaded")
	}

	e.emit(Progress{Phase: "delay", DelayTotal: len(ips)})
	pings := e.ping(ctx, ips)
	pings = filterAndSortPing(pings, e.cfg)
	if len(pings) == 0 {
		return nil, nil
	}
	e.emit(Progress{Phase: "download", DelayDone: len(ips), DelayTotal: len(ips), Available: len(pings), DownloadTotal: minInt(e.cfg.DownloadCandidates, len(pings))})
	results := e.download(ctx, pings, round)
	sort.Slice(results, func(i, j int) bool {
		if results[i].DownloadSpeed != results[j].DownloadSpeed {
			return results[i].DownloadSpeed > results[j].DownloadSpeed
		}
		return results[i].Delay < results[j].Delay
	})
	return results, nil
}

func (e *Engine) ping(ctx context.Context, ips []*net.IPAddr) []pingData {
	var wg sync.WaitGroup
	var mu sync.Mutex
	control := make(chan struct{}, e.cfg.DelayThreads)
	out := make([]pingData, 0)
	total := len(ips)
	done := 0
	available := 0

	for _, ip := range ips {
		select {
		case <-ctx.Done():
			return out
		default:
		}
		control <- struct{}{}
		wg.Add(1)
		go func(ip *net.IPAddr) {
			defer wg.Done()
			defer func() { <-control }()
			data := e.checkConnection(ctx, ip)
			mu.Lock()
			done++
			if data.received > 0 {
				available++
				out = append(out, data)
			}
			if done == total || done%25 == 0 {
				e.emit(Progress{Phase: "delay", DelayDone: done, DelayTotal: total, Available: available})
			}
			mu.Unlock()
		}(ip)
	}
	wg.Wait()
	return out
}

func (e *Engine) checkConnection(ctx context.Context, ip *net.IPAddr) pingData {
	if e.cfg.HTTPing {
		return e.httping(ctx, ip)
	}
	data := pingData{ip: ip, sent: e.cfg.PingTimes}
	for i := 0; i < e.cfg.PingTimes; i++ {
		ok, delay := e.tcping(ctx, ip)
		if ok {
			data.received++
			data.delay += delay
		}
	}
	if data.received > 0 {
		data.delay /= time.Duration(data.received)
	}
	return data
}

func (e *Engine) tcping(ctx context.Context, ip *net.IPAddr) (bool, time.Duration) {
	start := time.Now()
	dialer := net.Dialer{Timeout: tcpConnectTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addressFor(ip, e.cfg.Port))
	if err != nil {
		return false, 0
	}
	_ = conn.Close()
	return true, time.Since(start)
}

func (e *Engine) httping(ctx context.Context, ip *net.IPAddr) pingData {
	data := pingData{ip: ip, sent: e.cfg.PingTimes}
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			DialContext: getDialContext(ip, e.cfg.Port),
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	defer client.CloseIdleConnections()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, e.cfg.URL, nil)
	if err != nil {
		return data
	}
	setUserAgent(req)
	resp, err := client.Do(req)
	if err != nil {
		return data
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if !e.validStatusCode(resp.StatusCode) {
		return data
	}
	data.colo = getHeaderColo(resp.Header)
	if e.cfg.CFColo != "" && !matchColo(data.colo, e.cfg.CFColo) {
		return pingData{ip: ip, sent: e.cfg.PingTimes}
	}

	for i := 0; i < e.cfg.PingTimes; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, e.cfg.URL, nil)
		if err != nil {
			continue
		}
		setUserAgent(req)
		if i == e.cfg.PingTimes-1 {
			req.Header.Set("Connection", "close")
		}
		start := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		data.received++
		data.delay += time.Since(start)
	}
	if data.received > 0 {
		data.delay /= time.Duration(data.received)
	}
	return data
}

func (e *Engine) validStatusCode(code int) bool {
	if e.cfg.HTTPingStatusCode < 100 || e.cfg.HTTPingStatusCode > 599 {
		return code == 200 || code == 301 || code == 302
	}
	return code == e.cfg.HTTPingStatusCode
}

func filterAndSortPing(pings []pingData, cfg config.TestConfig) []pingData {
	sort.Slice(pings, func(i, j int) bool {
		lossI := lossRate(pings[i])
		lossJ := lossRate(pings[j])
		if lossI != lossJ {
			return lossI < lossJ
		}
		return pings[i].delay < pings[j].delay
	})
	filtered := make([]pingData, 0, len(pings))
	minDelay := time.Duration(cfg.MinDelayMS) * time.Millisecond
	maxDelay := time.Duration(cfg.MaxDelayMS) * time.Millisecond
	for _, p := range pings {
		if p.delay < minDelay || p.delay > maxDelay {
			continue
		}
		if lossRate(p) > cfg.MaxLossRate {
			continue
		}
		filtered = append(filtered, p)
	}
	return filtered
}

func (e *Engine) download(ctx context.Context, pings []pingData, round int) []model.Result {
	testNum := e.cfg.DownloadCandidates
	if testNum > len(pings) || e.cfg.MinSpeedMB > 0 {
		testNum = len(pings)
	}
	results := make([]model.Result, 0, testNum)
	for i := 0; i < testNum; i++ {
		select {
		case <-ctx.Done():
			return results
		default:
		}
		e.emit(Progress{Phase: "download", DelayDone: len(pings), DelayTotal: len(pings), Available: len(pings), DownloadDone: i, DownloadTotal: testNum, CurrentIP: pings[i].ip.String(), UsableResults: len(results)})
		speed, colo := e.downloadHandler(ctx, pings[i].ip)
		if pings[i].colo != "" {
			colo = pings[i].colo
		}
		if speed > 0 && speed >= e.cfg.MinSpeedMB*1024*1024 {
			results = append(results, toResult(pings[i], speed, colo, round))
		}
		if e.cfg.MinSpeedMB > 0 && len(results) >= e.cfg.DownloadCandidates {
			break
		}
		e.emit(Progress{Phase: "download", DelayDone: len(pings), DelayTotal: len(pings), Available: len(pings), DownloadDone: i + 1, DownloadTotal: testNum, LastIP: pings[i].ip.String(), LastSpeedMBps: speed / 1024 / 1024, UsableResults: len(results)})
	}
	return results
}

func (e *Engine) emit(progress Progress) {
	if e.onProgress != nil {
		e.onProgress(progress)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (e *Engine) downloadHandler(ctx context.Context, ip *net.IPAddr) (float64, string) {
	client := &http.Client{
		Transport: &http.Transport{DialContext: getDialContext(ip, e.cfg.Port)},
		Timeout:   e.cfg.DownloadTimeout(),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	defer client.CloseIdleConnections()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.cfg.URL, nil)
	if err != nil {
		return 0, ""
	}
	setUserAgent(req)
	resp, err := client.Do(req)
	if err != nil {
		return 0, ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, ""
	}
	colo := getHeaderColo(resp.Header)
	timeout := e.cfg.DownloadTimeout()
	timeStart := time.Now()
	timeEnd := timeStart.Add(timeout)
	contentLength := resp.ContentLength
	buffer := make([]byte, bufferSize)

	var (
		contentRead     int64
		timeSlice       = timeout / 100
		timeCounter     = 1
		lastContentRead int64
	)
	nextTime := timeStart.Add(timeSlice * time.Duration(timeCounter))
	avg := ewma.NewMovingAverage()

	for contentLength != contentRead {
		currentTime := time.Now()
		if currentTime.After(nextTime) {
			timeCounter++
			nextTime = timeStart.Add(timeSlice * time.Duration(timeCounter))
			avg.Add(float64(contentRead - lastContentRead))
			lastContentRead = contentRead
		}
		if currentTime.After(timeEnd) {
			break
		}
		n, err := resp.Body.Read(buffer)
		if err != nil {
			if err != io.EOF || contentLength == -1 {
				break
			}
			lastSlice := timeStart.Add(timeSlice * time.Duration(timeCounter-1))
			avg.Add(float64(contentRead-lastContentRead) / (float64(currentTime.Sub(lastSlice)) / float64(timeSlice)))
		}
		contentRead += int64(n)
	}
	elapsed := time.Since(timeStart).Seconds()
	if elapsed <= 0 {
		return 0, colo
	}
	speed := avg.Value() / (timeout.Seconds() / 120)
	fallback := float64(contentRead) / elapsed
	if speed <= 0 || fallback > speed*2 {
		speed = fallback
	}
	return speed, colo
}

func getDialContext(ip *net.IPAddr, port int) func(ctx context.Context, network, address string) (net.Conn, error) {
	target := addressFor(ip, port)
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, target)
	}
}

func addressFor(ip *net.IPAddr, port int) string {
	if isIPv4(ip.String()) {
		return fmt.Sprintf("%s:%d", ip.String(), port)
	}
	return fmt.Sprintf("[%s]:%d", ip.String(), port)
}

func setUserAgent(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; cfst-daemon/1.0)")
}

func lossRate(p pingData) float64 {
	if p.sent <= 0 {
		return 1
	}
	return float64(p.sent-p.received) / float64(p.sent)
}

func toResult(p pingData, speed float64, colo string, round int) model.Result {
	return model.Result{
		IP:            p.ip.String(),
		Sent:          p.sent,
		Received:      p.received,
		LossRate:      lossRate(p),
		Delay:         p.delay,
		DelayMS:       float64(p.delay.Microseconds()) / 1000,
		DownloadSpeed: speed,
		DownloadMBps:  speed / 1024 / 1024,
		Colo:          emptyNA(colo),
		Round:         round,
	}
}

func emptyNA(s string) string {
	if s == "" {
		return "N/A"
	}
	return s
}

func matchColo(colo, wanted string) bool {
	if colo == "" {
		return false
	}
	colo = strings.ToUpper(colo)
	for _, item := range strings.Split(strings.ToUpper(wanted), ",") {
		if strings.TrimSpace(item) == colo {
			return true
		}
	}
	return false
}

func getHeaderColo(header http.Header) string {
	if header.Get("server") != "" {
		if header.Get("server") == "cloudflare" {
			if colo := header.Get("cf-ray"); colo != "" {
				return regexpColoIATA.FindString(colo)
			}
		}
		if header.Get("server") == "CDN77-Turbo" {
			if colo := header.Get("x-77-pop"); colo != "" {
				return regexpColoCountry.FindString(colo)
			}
		}
		if colo := header.Get("server"); strings.Contains(colo, "BunnyCDN-") {
			return regexpColoCountry.FindString(strings.TrimPrefix(colo, "BunnyCDN-"))
		}
	}
	if colo := header.Get("x-amz-cf-pop"); colo != "" {
		return regexpColoIATA.FindString(colo)
	}
	if colo := header.Get("x-served-by"); colo != "" {
		if matches := regexpColoIATA.FindAllString(colo, -1); len(matches) > 0 {
			return matches[len(matches)-1]
		}
	}
	if colo := header.Get("x-id-fe"); colo != "" {
		if match := regexpColoGcore.FindString(colo); match != "" {
			return strings.ToUpper(match)
		}
	}
	return ""
}
