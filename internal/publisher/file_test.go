package publisher

import (
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/clover-eric/ato-cfip/internal/model"
)

func TestFormatCSV(t *testing.T) {
	set := model.PublishedSet{
		Domain:      "cf.example.com",
		GeneratedAt: time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC),
		IPs: []model.Result{
			{IP: "1.1.1.1", DownloadMBps: 42.345, DelayMS: 12.34, LossRate: 0.01, Colo: "SJC", Round: 3},
		},
	}
	rows, err := csv.NewReader(strings.NewReader(string(FormatCSV(set)))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected header and one data row, got %#v", rows)
	}
	if rows[0][0] != "rank" || rows[0][1] != "ip" {
		t.Fatalf("unexpected header: %#v", rows[0])
	}
	if rows[1][0] != "1" || rows[1][1] != "1.1.1.1" || rows[1][2] != "cf.example.com" {
		t.Fatalf("unexpected data row: %#v", rows[1])
	}
}
