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
		r.setProgress(round, len(selected), len(all), "Scanning candidate IPs")
		results, err := engine.New(r.cfg.Test).Run(ctx, round)
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
		all = append(all, results...)
		winner := firstNewWinner(results, selected)
		if winner == nil {
			log.Printf("round %d winner duplicated, no new unique IP found in candidate list", round)
			r.setProgress(round, len(selected), len(all), fmt.Sprintf("Round %d was duplicated, continuing", round))
			continue
		}
		selected[winner.IP] = *winner
		stage := fmt.Sprintf("Selected %d/%d IPs", len(selected), r.cfg.Test.DesiredUniqueIPs)
		if zeroSpeed > 0 {
			stage = fmt.Sprintf("%s; %d candidates had 0 MB/s download", stage, zeroSpeed)
		}
		r.setProgress(round, len(selected), len(all), stage)
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
	if status.Running && status.LastStarted != nil {
		status.ElapsedSec = int64(time.Since(*status.LastStarted).Seconds())
	}
	return status
}

func (r *Runner) Config() config.Config {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cfg
}

func (r *Runner) SetDomain(domain string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfg.Publish.Domain = domain
	r.lastRun.Published.Domain = domain
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
}

func (r *Runner) setProgress(round, selected, candidates int, stage string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastRun.CurrentRound = round
	r.lastRun.Selected = selected
	r.lastRun.Candidates = candidates
	r.lastRun.Stage = stage
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
