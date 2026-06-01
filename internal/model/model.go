package model

import "time"

type Result struct {
	IP            string        `json:"ip"`
	Sent          int           `json:"sent"`
	Received      int           `json:"received"`
	LossRate      float64       `json:"loss_rate"`
	Delay         time.Duration `json:"-"`
	DelayMS       float64       `json:"delay_ms"`
	DownloadSpeed float64       `json:"-"`
	DownloadMBps  float64       `json:"download_mbps"`
	Colo          string        `json:"colo"`
	Round         int           `json:"round"`
}

type PublishedSet struct {
	Domain      string    `json:"domain"`
	GeneratedAt time.Time `json:"generated_at"`
	IPs         []Result  `json:"ips"`
}
