package publisher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/clover-eric/ato-cfip/internal/model"
)

type CloudflarePublisher struct {
	APIToken string
	ZoneID   string
	Domain   string
	TTL      int
	Proxied  bool
}

type cloudflareRecord struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

type cloudflareListResponse struct {
	Success bool               `json:"success"`
	Errors  []cloudflareError  `json:"errors"`
	Result  []cloudflareRecord `json:"result"`
}

type cloudflareMutationResponse struct {
	Success bool              `json:"success"`
	Errors  []cloudflareError `json:"errors"`
}

type cloudflareError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (p CloudflarePublisher) Publish(ctx context.Context, set model.PublishedSet) error {
	records, err := p.listRecords(ctx)
	if err != nil {
		return err
	}
	desired := make(map[string]cloudflareRecord)
	for _, item := range set.IPs {
		recordType := "A"
		if net.ParseIP(item.IP).To4() == nil {
			recordType = "AAAA"
		}
		desired[recordType+"|"+item.IP] = cloudflareRecord{
			Type:    recordType,
			Name:    p.Domain,
			Content: item.IP,
			TTL:     p.TTL,
			Proxied: p.Proxied,
		}
	}
	for _, record := range records {
		key := record.Type + "|" + record.Content
		if _, ok := desired[key]; ok {
			delete(desired, key)
			continue
		}
		if err := p.deleteRecord(ctx, record.ID); err != nil {
			return err
		}
	}
	for _, record := range desired {
		if err := p.createRecord(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func (p CloudflarePublisher) listRecords(ctx context.Context) ([]cloudflareRecord, error) {
	endpoint := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records?name=%s&per_page=100", p.ZoneID, url.QueryEscape(p.Domain))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	p.authorize(req)
	var out cloudflareListResponse
	if err := doJSON(req, nil, &out); err != nil {
		return nil, err
	}
	if !out.Success {
		return nil, fmt.Errorf("cloudflare list records failed: %v", out.Errors)
	}
	filtered := make([]cloudflareRecord, 0)
	for _, record := range out.Result {
		if record.Type == "A" || record.Type == "AAAA" {
			filtered = append(filtered, record)
		}
	}
	return filtered, nil
}

func (p CloudflarePublisher) createRecord(ctx context.Context, record cloudflareRecord) error {
	endpoint := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", p.ZoneID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	p.authorize(req)
	var out cloudflareMutationResponse
	if err := doJSON(req, record, &out); err != nil {
		return err
	}
	if !out.Success {
		return fmt.Errorf("cloudflare create record failed: %v", out.Errors)
	}
	return nil
}

func (p CloudflarePublisher) deleteRecord(ctx context.Context, id string) error {
	endpoint := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", p.ZoneID, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	p.authorize(req)
	var out cloudflareMutationResponse
	if err := doJSON(req, nil, &out); err != nil {
		return err
	}
	if !out.Success {
		return fmt.Errorf("cloudflare delete record failed: %v", out.Errors)
	}
	return nil
}

func (p CloudflarePublisher) authorize(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+p.APIToken)
	req.Header.Set("Content-Type", "application/json")
}

func doJSON(req *http.Request, body any, out any) error {
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		req.Body = io.NopCloser(bytes.NewReader(b))
		req.ContentLength = int64(len(b))
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cloudflare API returned %s: %s", resp.Status, string(b))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
