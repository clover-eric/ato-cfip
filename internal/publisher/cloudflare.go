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
	"strings"
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

type CloudflareZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type cloudflareZoneResponse struct {
	Success bool              `json:"success"`
	Errors  []cloudflareError `json:"errors"`
	Result  []CloudflareZone  `json:"result"`
}

type cloudflareListResponse struct {
	Success bool               `json:"success"`
	Errors  []cloudflareError  `json:"errors"`
	Result  []cloudflareRecord `json:"result"`
}

type cloudflareMutationResponse struct {
	Success bool              `json:"success"`
	Errors  []cloudflareError `json:"errors"`
	Result  cloudflareRecord  `json:"result"`
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
		if wanted, ok := desired[key]; ok {
			wanted.ID = record.ID
			if needsRecordUpdate(record, wanted) {
				if err := p.updateRecord(ctx, record.ID, wanted); err != nil {
					return err
				}
			}
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

func needsRecordUpdate(existing, desired cloudflareRecord) bool {
	if !strings.EqualFold(existing.Type, desired.Type) {
		return true
	}
	if !strings.EqualFold(existing.Name, desired.Name) {
		return true
	}
	if existing.Content != desired.Content {
		return true
	}
	if existing.Proxied != desired.Proxied {
		return true
	}
	if !desired.Proxied && existing.TTL != desired.TTL {
		return true
	}
	return false
}

func FindZone(ctx context.Context, apiToken, domain string) (CloudflareZone, error) {
	labels := strings.Split(strings.Trim(strings.ToLower(domain), "."), ".")
	if len(labels) < 2 {
		return CloudflareZone{}, fmt.Errorf("domain must contain at least two labels")
	}
	for i := 0; i < len(labels)-1; i++ {
		name := strings.Join(labels[i:], ".")
		zone, err := listZoneByName(ctx, apiToken, name)
		if err != nil {
			return CloudflareZone{}, err
		}
		if zone.ID != "" {
			return zone, nil
		}
	}
	return CloudflareZone{}, fmt.Errorf("no matching Cloudflare zone found for %s", domain)
}

func UpsertRecords(ctx context.Context, apiToken, zoneID, domain string, ips []string, ttl int, proxied bool) error {
	if ttl <= 0 {
		ttl = 60
	}
	pub := CloudflarePublisher{APIToken: apiToken, ZoneID: zoneID, Domain: domain, TTL: ttl, Proxied: proxied}
	set := model.PublishedSet{Domain: domain}
	for _, ip := range ips {
		if strings.TrimSpace(ip) == "" {
			continue
		}
		set.IPs = append(set.IPs, model.Result{IP: strings.TrimSpace(ip)})
	}
	return pub.Publish(ctx, set)
}

func listZoneByName(ctx context.Context, apiToken, name string) (CloudflareZone, error) {
	endpoint := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones?name=%s&per_page=1", url.QueryEscape(name))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return CloudflareZone{}, err
	}
	authorizeCloudflare(req, apiToken)
	var out cloudflareZoneResponse
	if err := doJSON(req, nil, &out); err != nil {
		return CloudflareZone{}, err
	}
	if !out.Success {
		return CloudflareZone{}, fmt.Errorf("cloudflare list zones failed: %v", out.Errors)
	}
	if len(out.Result) == 0 {
		return CloudflareZone{}, nil
	}
	return out.Result[0], nil
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

func (p CloudflarePublisher) updateRecord(ctx context.Context, id string, record cloudflareRecord) error {
	record.ID = ""
	endpoint := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", p.ZoneID, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, nil)
	if err != nil {
		return err
	}
	p.authorize(req)
	var out cloudflareMutationResponse
	if err := doJSON(req, record, &out); err != nil {
		return err
	}
	if !out.Success {
		return fmt.Errorf("cloudflare update record failed: %v", out.Errors)
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
	authorizeCloudflare(req, p.APIToken)
}

func authorizeCloudflare(req *http.Request, apiToken string) {
	req.Header.Set("Authorization", "Bearer "+apiToken)
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
