package config

import (
	"errors"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Web     WebConfig     `yaml:"web"`
	Test    TestConfig    `yaml:"test"`
	Publish PublishConfig `yaml:"publish"`
	Storage StorageConfig `yaml:"storage"`
	Log     LogConfig     `yaml:"log"`
}

type ServerConfig struct {
	RunOnStart bool   `yaml:"run_on_start"`
	Schedule   string `yaml:"schedule"`
	Timezone   string `yaml:"timezone"`
}

type WebConfig struct {
	Enabled bool   `yaml:"enabled"`
	Listen  string `yaml:"listen"`
	Title   string `yaml:"title"`
}

type TestConfig struct {
	RoundsPerHour       int     `yaml:"rounds_per_hour"`
	DesiredUniqueIPs    int     `yaml:"desired_unique_ips"`
	MaxExtraRounds      int     `yaml:"max_extra_rounds"`
	IPFile              string  `yaml:"ip_file"`
	IPText              string  `yaml:"ip_text"`
	IPv6                bool    `yaml:"ipv6"`
	AllIP               bool    `yaml:"all_ip"`
	Port                int     `yaml:"port"`
	URL                 string  `yaml:"url"`
	DelayThreads        int     `yaml:"delay_threads"`
	PingTimes           int     `yaml:"ping_times"`
	DownloadTimeSeconds int     `yaml:"download_time_seconds"`
	DownloadCandidates  int     `yaml:"download_candidates"`
	MinDelayMS          int     `yaml:"min_delay_ms"`
	MaxDelayMS          int     `yaml:"max_delay_ms"`
	MaxLossRate         float64 `yaml:"max_loss_rate"`
	MinSpeedMB          float64 `yaml:"min_speed_mb"`
	HTTPing             bool    `yaml:"httping"`
	HTTPingStatusCode   int     `yaml:"httping_status_code"`
	CFColo              string  `yaml:"cfcolo"`
}

type PublishConfig struct {
	Mode       string           `yaml:"mode"`
	Domain     string           `yaml:"domain"`
	TTL        int              `yaml:"ttl"`
	OutputFile string           `yaml:"output_file"`
	HostsFile  string           `yaml:"hosts_file"`
	Cloudflare CloudflareConfig `yaml:"cloudflare"`
}

type CloudflareConfig struct {
	APIToken string `yaml:"api_token"`
	ZoneID   string `yaml:"zone_id"`
	Proxied  bool   `yaml:"proxied"`
}

type StorageConfig struct {
	HistoryPath string `yaml:"history_path"`
	KeepDays    int    `yaml:"keep_days"`
}

type LogConfig struct {
	Verbose bool `yaml:"verbose"`
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	return cfg, cfg.Validate()
}

func Defaults() Config {
	return Config{
		Server: ServerConfig{
			RunOnStart: true,
			Schedule:   "0 * * * *",
			Timezone:   "Asia/Shanghai",
		},
		Web: WebConfig{
			Enabled: true,
			Listen:  ":8080",
			Title:   "CFST 优选节点",
		},
		Test: TestConfig{
			RoundsPerHour:       10,
			DesiredUniqueIPs:    10,
			MaxExtraRounds:      30,
			IPFile:              "ip.txt",
			Port:                443,
			URL:                 "https://cf.xiu2.xyz/url",
			DelayThreads:        200,
			PingTimes:           4,
			DownloadTimeSeconds: 10,
			DownloadCandidates:  10,
			MaxDelayMS:          9999,
			MaxLossRate:         1.0,
		},
		Publish: PublishConfig{
			Mode:       "file",
			Domain:     "best.example.com",
			TTL:        60,
			OutputFile: "data/best_ips.json",
			HostsFile:  "data/hosts.txt",
		},
		Storage: StorageConfig{
			HistoryPath: "data/results.jsonl",
			KeepDays:    30,
		},
		Log: LogConfig{Verbose: true},
	}
}

func (c Config) Validate() error {
	if c.Test.RoundsPerHour <= 0 {
		return errors.New("test.rounds_per_hour must be greater than 0")
	}
	if c.Test.DesiredUniqueIPs <= 0 {
		return errors.New("test.desired_unique_ips must be greater than 0")
	}
	if c.Test.DelayThreads <= 0 {
		return errors.New("test.delay_threads must be greater than 0")
	}
	if c.Test.Port <= 0 || c.Test.Port > 65535 {
		return errors.New("test.port must be between 1 and 65535")
	}
	if c.Test.URL == "" {
		return errors.New("test.url is required")
	}
	if c.Publish.Domain == "" {
		return errors.New("publish.domain is required")
	}
	if c.Publish.TTL <= 0 {
		return errors.New("publish.ttl must be greater than 0")
	}
	if c.Web.Enabled && c.Web.Listen == "" {
		return errors.New("web.listen is required when web.enabled is true")
	}
	return nil
}

func (c TestConfig) DownloadTimeout() time.Duration {
	if c.DownloadTimeSeconds <= 0 {
		return 10 * time.Second
	}
	return time.Duration(c.DownloadTimeSeconds) * time.Second
}
