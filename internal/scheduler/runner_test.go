package scheduler

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/clover-eric/ato-cfip/internal/config"
	"github.com/clover-eric/ato-cfip/internal/engine"
	"github.com/clover-eric/ato-cfip/internal/model"
	"github.com/clover-eric/ato-cfip/internal/store"
)

func TestFilterQualifiedUsesSpeedAndDelay(t *testing.T) {
	cfg := config.TestConfig{
		MinSpeedMB:  30,
		MaxDelayMS:  60,
		MaxLossRate: 0,
	}
	results := []model.Result{
		{IP: "1.1.1.1", DownloadMBps: 31, DelayMS: 59, LossRate: 0},
		{IP: "1.1.1.2", DownloadMBps: 29, DelayMS: 59, LossRate: 0},
		{IP: "1.1.1.3", DownloadMBps: 31, DelayMS: 61, LossRate: 0},
		{IP: "1.1.1.4", DownloadMBps: 31, DelayMS: 59, LossRate: 0.5},
	}
	got := filterQualified(results, cfg)
	if len(got) != 1 || got[0].IP != "1.1.1.1" {
		t.Fatalf("unexpected qualified results: %#v", got)
	}
}

func TestResultIPsDeduplicatesPool(t *testing.T) {
	results := []model.Result{
		{IP: "1.1.1.1"},
		{IP: "1.1.1.1"},
		{IP: "1.1.1.2"},
		{IP: ""},
	}
	got := resultIPs(results)
	if len(got) != 2 || got[0] != "1.1.1.1" || got[1] != "1.1.1.2" {
		t.Fatalf("unexpected ip list: %#v", got)
	}
}

func TestRunOnceKeepsHealthyPoolWithoutPublicScan(t *testing.T) {
	st := openTestStore(t)
	cfg := runnerTestConfig(t)
	previous := []model.Result{
		qualifiedResult("1.1.1.1"),
		qualifiedResult("1.1.1.2"),
	}
	if err := st.SaveCycle(context.Background(), time.Now(), previous, previous); err != nil {
		t.Fatal(err)
	}
	pub := &fakePublisher{}
	r := NewRunner(cfg, st, pub)
	fake := &fakeSpeedEngine{runIPsResults: previous}
	r.engineFactory = func(config.TestConfig, func(engine.Progress)) speedEngine { return fake }

	if err := r.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fake.runCalled != 0 {
		t.Fatalf("public scan should not be called for healthy full pool, called %d", fake.runCalled)
	}
	if fake.runIPsCalled != 1 {
		t.Fatalf("pool recheck should be called once, called %d", fake.runIPsCalled)
	}
	if len(pub.last.IPs) != 2 {
		t.Fatalf("expected full pool to be published, got %#v", pub.last.IPs)
	}
}

func TestRunOnceReplacesOnlyMissingPoolSlots(t *testing.T) {
	st := openTestStore(t)
	cfg := runnerTestConfig(t)
	previous := []model.Result{
		qualifiedResult("1.1.1.1"),
		qualifiedResult("1.1.1.2"),
	}
	if err := st.SaveCycle(context.Background(), time.Now(), previous, previous); err != nil {
		t.Fatal(err)
	}
	replacement := qualifiedResult("1.1.1.3")
	pub := &fakePublisher{}
	r := NewRunner(cfg, st, pub)
	fake := &fakeSpeedEngine{
		runIPsResults: []model.Result{qualifiedResult("1.1.1.1")},
		runResults:    []model.Result{replacement},
	}
	r.engineFactory = func(config.TestConfig, func(engine.Progress)) speedEngine { return fake }

	if err := r.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fake.runIPsCalled != 1 || fake.runCalled != 1 {
		t.Fatalf("expected one pool recheck and one public scan, got pool=%d scan=%d", fake.runIPsCalled, fake.runCalled)
	}
	ips := resultIPs(pub.last.IPs)
	if len(ips) != 2 || !containsString(ips, "1.1.1.1") || !containsString(ips, "1.1.1.3") {
		t.Fatalf("unexpected published replacement pool: %#v", pub.last.IPs)
	}
}

func runnerTestConfig(t *testing.T) config.Config {
	t.Helper()
	return config.Config{
		Test: config.TestConfig{
			RoundsPerHour:    1,
			DesiredUniqueIPs: 2,
			MaxExtraRounds:   0,
			MinSpeedMB:       30,
			MaxDelayMS:       60,
			MaxLossRate:      0,
		},
		Publish: config.PublishConfig{
			Domain: "cf.example.net",
			TTL:    60,
		},
		Storage: config.StorageConfig{
			HistoryPath: filepath.Join(t.TempDir(), "unused.jsonl"),
			KeepDays:    0,
		},
	}
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "results.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func qualifiedResult(ip string) model.Result {
	return model.Result{
		IP:            ip,
		Received:      3,
		Sent:          3,
		DelayMS:       50,
		DownloadMBps:  35,
		DownloadSpeed: 35 * 1024 * 1024,
		Colo:          "TEST",
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

type fakeSpeedEngine struct {
	runCalled     int
	runIPsCalled  int
	runResults    []model.Result
	runIPsResults []model.Result
}

func (e *fakeSpeedEngine) Run(context.Context, int) ([]model.Result, error) {
	e.runCalled++
	return e.runResults, nil
}

func (e *fakeSpeedEngine) RunIPs(context.Context, []string, int) ([]model.Result, error) {
	e.runIPsCalled++
	return e.runIPsResults, nil
}

type fakePublisher struct {
	last model.PublishedSet
}

func (p *fakePublisher) Publish(_ context.Context, set model.PublishedSet) error {
	p.last = set
	return nil
}
