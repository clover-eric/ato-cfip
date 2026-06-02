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
	cfg           config.Config
	store         *store.Store
	publisher     publisher.Publisher
	engineFactory engineFactory
	mu            sync.Mutex
	running       bool
	lastRun       RunStatus
}

type speedEngine interface {
	Run(ctx context.Context, round int) ([]model.Result, error)
	RunIPs(ctx context.Context, candidates []string, round int) ([]model.Result, error)
}

type engineFactory func(config.TestConfig, func(engine.Progress)) speedEngine

type RunStatus struct {
	Running      bool               `json:"running"`
	LastStarted  *time.Time         `json:"last_started,omitempty"`
	LastEnded    *time.Time         `json:"last_ended,omitempty"`
	LastError    string             `json:"last_error,omitempty"`
	Published    model.PublishedSet `json:"published"`
	Rounds       int                `json:"rounds"`
	Target       int                `json:"target"`
	CurrentRound int                `json:"current_round"`
	Selected     int                `json:"selected"`
	Candidates   int                `json:"candidates"`
	Stage        string             `json:"stage"`
	ElapsedSec   int64              `json:"elapsed_sec"`
	ZeroSpeed    int                `json:"zero_speed"`
	Progress     engine.Progress    `json:"progress"`
	ArchiveCount int                `json:"archive_count"`
}

func NewRunner(cfg config.Config, st *store.Store, pub publisher.Publisher) *Runner {
	r := &Runner{cfg: cfg, store: st, publisher: pub, engineFactory: defaultEngineFactory}
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

func defaultEngineFactory(cfg config.TestConfig, progress func(engine.Progress)) speedEngine {
	return engine.New(cfg).WithProgress(progress)
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
	log.Printf("starting speed-test cycle: rounds=%d target_unique=%d min_speed=%.1fMB/s max_delay=%dms", r.cfg.Test.RoundsPerHour, r.cfg.Test.DesiredUniqueIPs, r.cfg.Test.MinSpeedMB, r.cfg.Test.MaxDelayMS)
	previous := r.publishedSnapshot()
	selected := make(map[string]model.Result)
	all := make([]model.Result, 0)
	totalRounds := r.cfg.Test.RoundsPerHour + r.cfg.Test.MaxExtraRounds
	if totalRounds < r.cfg.Test.RoundsPerHour {
		totalRounds = r.cfg.Test.RoundsPerHour
	}

	if len(previous) > 0 {
		poolIPs := resultIPs(previous)
		r.setProgress(0, len(selected), len(all), fmt.Sprintf("Rechecking stable IP pool %d/%d", len(poolIPs), r.cfg.Test.DesiredUniqueIPs))
		results, err := r.engineFactory(r.cfg.Test, func(progress engine.Progress) {
			r.setEngineProgress(0, len(selected), len(all), progress)
		}).RunIPs(ctx, poolIPs, 0)
		if err != nil {
			log.Printf("pool recheck failed: %v", err)
			r.setProgress(0, len(selected), len(all), fmt.Sprintf("Pool recheck failed: %v", err))
		} else {
			results = filterQualified(results, r.cfg.Test)
			all = append(all, results...)
			added := addNewWinners(results, selected, r.cfg.Test.DesiredUniqueIPs)
			dropped := len(poolIPs) - added
			if dropped < 0 {
				dropped = 0
			}
			r.setProgress(0, len(selected), len(all), fmt.Sprintf("Pool recheck kept %d/%d IPs, %d need replacement", added, len(poolIPs), dropped))
			log.Printf("pool recheck kept=%d dropped=%d target=%d", added, dropped, r.cfg.Test.DesiredUniqueIPs)
		}
	}

	if len(selected) < r.cfg.Test.DesiredUniqueIPs {
		for round := 1; round <= totalRounds; round++ {
			if round > r.cfg.Test.RoundsPerHour && len(selected) >= r.cfg.Test.DesiredUniqueIPs {
				break
			}
			needed := r.cfg.Test.DesiredUniqueIPs - len(selected)
			r.setProgress(round, len(selected), len(all), fmt.Sprintf("Scanning public IP library, need %d replacements", needed))
			results, err := r.engineFactory(r.cfg.Test, func(progress engine.Progress) {
				r.setEngineProgress(round, len(selected), len(all), progress)
			}).Run(ctx, round)
			if err != nil {
				log.Printf("round %d failed: %v", round, err)
				r.setProgress(round, len(selected), len(all), fmt.Sprintf("Round %d failed: %v", round, err))
				continue
			}
			if len(results) == 0 {
				log.Printf("round %d produced no usable result", round)
				r.setProgress(round, len(selected), len(all), fmt.Sprintf("Round %d produced no usable result", round))
				continue
			}
			zeroSpeed := countZeroSpeed(results)
			if zeroSpeed > 0 {
				r.addZeroSpeed(zeroSpeed)
			}
			results = filterQualified(results, r.cfg.Test)
			all = append(all, results...)
			if len(results) == 0 {
				log.Printf("round %d produced no qualified result", round)
				r.setProgress(round, len(selected), len(all), fmt.Sprintf("Round %d produced no IP above %.1f MB/s and below %d ms", round, r.cfg.Test.MinSpeedMB, r.cfg.Test.MaxDelayMS))
				continue
			}
			added := addNewWinners(results, selected, r.cfg.Test.DesiredUniqueIPs)
			if added == 0 {
				log.Printf("round %d results duplicated, no new unique IP found in candidate list", round)
				r.setProgress(round, len(selected), len(all), fmt.Sprintf("Round %d was duplicated, continuing", round))
				continue
			}
			stage := fmt.Sprintf("Selected %d/%d qualified IPs", len(selected), r.cfg.Test.DesiredUniqueIPs)
			if zeroSpeed > 0 {
				stage = fmt.Sprintf("%s; %d candidates had 0 MB/s download", stage, zeroSpeed)
			}
			r.setProgress(round, len(selected), len(all), stage)
			log.Printf("round %d added %d IPs, selected=%d/%d", round, added, len(selected), r.cfg.Test.DesiredUniqueIPs)
		}
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
	if len(final) < r.cfg.Test.DesiredUniqueIPs {
		runErr = fmt.Errorf("qualified IP pool incomplete: %d/%d IPs meet speed >= %.1f MB/s and delay <= %d ms; keeping previous published pool", len(final), r.cfg.Test.DesiredUniqueIPs, r.cfg.Test.MinSpeedMB, r.cfg.Test.MaxDelayMS)
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
	if status.Running && status.LastStarted != nil {
		status.ElapsedSec = int64(time.Since(*status.LastStarted).Seconds())
	}
	status.ArchiveCount = len(status.Published.IPs)
	return status
}

func (r *Runner) Config() config.Config {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cfg
}

func (r *Runner) publishedSnapshot() []model.Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]model.Result, len(r.lastRun.Published.IPs))
	copy(out, r.lastRun.Published.IPs)
	return out
}

func (r *Runner) ArchiveStats() (store.Stats, error) {
	return r.store.Stats()
}

func (r *Runner) SetDomain(domain string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfg.Publish.Domain = domain
	r.lastRun.Published.Domain = domain
}

func (r *Runner) SetPublisher(cfg config.PublishConfig, pub publisher.Publisher) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfg.Publish = cfg
	r.publisher = pub
	r.lastRun.Published.Domain = cfg.Domain
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
	r.lastRun.CurrentRound = 0
	r.lastRun.Selected = 0
	r.lastRun.Candidates = 0
	r.lastRun.Stage = "Preparing"
	r.lastRun.ZeroSpeed = 0
	r.lastRun.Progress = engine.Progress{}
}

func (r *Runner) setProgress(round, selected, candidates int, stage string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastRun.CurrentRound = round
	r.lastRun.Selected = selected
	r.lastRun.Candidates = candidates
	r.lastRun.Stage = stage
}

func (r *Runner) setEngineProgress(round, selected, candidates int, progress engine.Progress) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastRun.CurrentRound = round
	r.lastRun.Selected = selected
	r.lastRun.Candidates = candidates
	r.lastRun.Progress = progress
	switch progress.Phase {
	case "delay":
		r.lastRun.Stage = fmt.Sprintf("Delay test %d/%d, available %d", progress.DelayDone, progress.DelayTotal, progress.Available)
	case "download":
		r.lastRun.Stage = fmt.Sprintf("Download test %d/%d", progress.DownloadDone, progress.DownloadTotal)
	}
}

func (r *Runner) addZeroSpeed(count int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastRun.ZeroSpeed += count
}

func (r *Runner) setPublished(set model.PublishedSet) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastRun.Published = set
	r.lastRun.Selected = len(set.IPs)
}

func countZeroSpeed(results []model.Result) int {
	var count int
	for _, result := range results {
		if result.DownloadMBps <= 0 {
			count++
		}
	}
	return count
}

func filterQualified(results []model.Result, cfg config.TestConfig) []model.Result {
	filtered := make([]model.Result, 0, len(results))
	for _, result := range results {
		if isQualified(result, cfg) {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

func isQualified(result model.Result, cfg config.TestConfig) bool {
	if result.DownloadMBps <= 0 || result.DownloadMBps < cfg.MinSpeedMB {
		return false
	}
	if cfg.MaxDelayMS > 0 && result.DelayMS > float64(cfg.MaxDelayMS) {
		return false
	}
	if cfg.MinDelayMS > 0 && result.DelayMS < float64(cfg.MinDelayMS) {
		return false
	}
	if cfg.MaxLossRate >= 0 && result.LossRate > cfg.MaxLossRate {
		return false
	}
	return true
}

func resultIPs(results []model.Result) []string {
	seen := make(map[string]struct{})
	ips := make([]string, 0, len(results))
	for _, result := range results {
		if result.IP == "" {
			continue
		}
		if _, ok := seen[result.IP]; ok {
			continue
		}
		seen[result.IP] = struct{}{}
		ips = append(ips, result.IP)
	}
	return ips
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
		r.lastRun.Stage = err.Error()
	} else {
		r.lastRun.Stage = "Finished"
	}
}

func addNewWinners(results []model.Result, selected map[string]model.Result, target int) int {
	added := 0
	for _, result := range results {
		if _, exists := selected[result.IP]; exists {
			continue
		}
		selected[result.IP] = result
		added++
		if len(selected) >= target {
			break
		}
	}
	return added
}
