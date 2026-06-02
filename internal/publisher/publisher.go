package publisher

import (
	"context"
	"fmt"
	"strings"

	"github.com/clover-eric/ato-cfip/internal/config"
	"github.com/clover-eric/ato-cfip/internal/model"
)

type Publisher interface {
	Publish(ctx context.Context, set model.PublishedSet) error
}

func New(cfg config.PublishConfig) (Publisher, error) {
	switch strings.ToLower(cfg.Mode) {
	case "", "file":
		return FilePublisher{OutputFile: cfg.OutputFile, HostsFile: cfg.HostsFile, CSVFile: cfg.CSVFile}, nil
	case "cloudflare", "cloudflare-dns":
		if cfg.Cloudflare.APIToken == "" || cfg.Cloudflare.ZoneID == "" {
			return nil, fmt.Errorf("cloudflare publisher requires api_token and zone_id")
		}
		return CloudflarePublisher{
			APIToken: cfg.Cloudflare.APIToken,
			ZoneID:   cfg.Cloudflare.ZoneID,
			Domain:   cfg.Domain,
			TTL:      cfg.TTL,
			Proxied:  cfg.Cloudflare.Proxied,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported publish mode %q", cfg.Mode)
	}
}
