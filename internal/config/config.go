package config

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
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
	cfg.ApplyEnv()
	cfg.ApplyRuntime()
	return cfg, cfg.Validate()
}

type RuntimeConfig struct {
	Initialized   bool   `json:"initialized"`
	AdminUser     string `json:"admin_user"`
	PasswordHash  string `json:"password_hash"`
	SessionSecret string `json:"session_secret"`
	PanelURL      string `json:"panel_url"`
	Domain        string `json:"domain"`
}

func (c *Config) ApplyRuntime() {
	b, err := os.ReadFile("data/runtime.json")
	if err != nil {
		return
	}
	var runtime RuntimeConfig
	if err := json.Unmarshal(b, &runtime); err != nil {
		return
	}
	if runtime.Domain != "" {
		c.Publish.Domain = runtime.Domain
	}
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
			Title:   "ATO CFIP",
		},
		Test: TestConfig{
			RoundsPerHour:       10,
			DesiredUniqueIPs:    10,
			MaxExtraRounds:      30,
			IPFile:              "ip.txt",
			Port:                443,
			URL:                 "https://speed.cloudflare.com/__down?bytes=50000000",
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

func (c *Config) ApplyEnv() {
	envString("ATO_WEB_TITLE", &c.Web.Title)
	envString("ATO_WEB_LISTEN", &c.Web.Listen)
	envString("ATO_SCHEDULE", &c.Server.Schedule)
	envString("ATO_TIMEZONE", &c.Server.Timezone)
	envBool("ATO_RUN_ON_START", &c.Server.RunOnStart)

	envString("ATO_DOMAIN", &c.Publish.Domain)
	envString("ATO_PUBLISH_MODE", &c.Publish.Mode)
	envString("ATO_OUTPUT_FILE", &c.Publish.OutputFile)
	envString("ATO_HOSTS_FILE", &c.Publish.HostsFile)
	envString("CLOUDFLARE_API_TOKEN", &c.Publish.Cloudflare.APIToken)
	envString("CLOUDFLARE_ZONE_ID", &c.Publish.Cloudflare.ZoneID)
	envBool("CLOUDFLARE_PROXIED", &c.Publish.Cloudflare.Proxied)

	envString("ATO_TEST_URL", &c.Test.URL)
	envString("ATO_IP_TEXT", &c.Test.IPText)
	envString("ATO_IP_FILE", &c.Test.IPFile)
	envInt("ATO_ROUNDS", &c.Test.RoundsPerHour)
	envInt("ATO_TARGET_IPS", &c.Test.DesiredUniqueIPs)
	envInt("ATO_MAX_EXTRA_ROUNDS", &c.Test.MaxExtraRounds)
	envInt("ATO_DOWNLOAD_SECONDS", &c.Test.DownloadTimeSeconds)
	envInt("ATO_DOWNLOAD_CANDIDATES", &c.Test.DownloadCandidates)
	envInt("ATO_DELAY_THREADS", &c.Test.DelayThreads)
	envInt("ATO_PING_TIMES", &c.Test.PingTimes)
	envInt("ATO_PORT", &c.Test.Port)
	envBool("ATO_IPV6", &c.Test.IPv6)
	envBool("ATO_ALL_IP", &c.Test.AllIP)
	envBool("ATO_HTTPING", &c.Test.HTTPing)
	envFloat("ATO_MIN_SPEED_MB", &c.Test.MinSpeedMB)
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

func envString(name string, target *string) {
	if v := os.Getenv(name); v != "" {
		*target = v
	}
}

func envInt(name string, target *int) {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			*target = n
		}
	}
}

func envBool(name string, target *bool) {
	if v := os.Getenv(name); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			*target = b
		}
	}
}

func envFloat(name string, target *float64) {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			*target = n
		}
	}
}
