package subscription

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const maxSubscriptionBytes = 8 << 20

type RenderResult struct {
	Content    string `json:"content"`
	Converted  int    `json:"converted"`
	Encoding   string `json:"encoding"`
	SourceType string `json:"source_type"`
}

func Render(ctx context.Context, entry Entry, preferredDomain string) (RenderResult, error) {
	preferredDomain = strings.TrimSpace(preferredDomain)
	if preferredDomain == "" || strings.ContainsAny(preferredDomain, "/?#@") {
		return RenderResult{}, fmt.Errorf("preferred domain is not configured")
	}
	source := entry.Source
	if entry.SourceType == "url" || DetectSourceType(source) == "url" {
		body, err := fetchSubscription(ctx, source)
		if err != nil {
			return RenderResult{}, err
		}
		source = body
	}
	result, err := Convert(source, preferredDomain)
	if err != nil {
		return RenderResult{}, err
	}
	result.SourceType = entry.SourceType
	return result, nil
}

func Convert(raw, preferredDomain string) (RenderResult, error) {
	if decoded, ok := decodeBase64Subscription(raw); ok {
		inner := convertPlain(decoded, preferredDomain)
		return RenderResult{
			Content:   base64.StdEncoding.EncodeToString([]byte(inner.Content)),
			Converted: inner.Converted,
			Encoding:  "base64",
		}, nil
	}
	return convertPlain(raw, preferredDomain), nil
}

func fetchSubscription(ctx context.Context, source string) (string, error) {
	source = strings.TrimSpace(source)
	u, err := url.Parse(source)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("invalid subscription URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "ATO-CFIP/1.0")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("subscription URL returned %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxSubscriptionBytes+1))
	if err != nil {
		return "", err
	}
	if len(b) > maxSubscriptionBytes {
		return "", fmt.Errorf("subscription content is too large")
	}
	return string(b), nil
}

func convertPlain(raw, preferredDomain string) RenderResult {
	trimmed := strings.TrimSpace(strings.TrimPrefix(raw, "\ufeff"))
	if converted, count, ok := convertJSON(trimmed, preferredDomain); ok {
		return RenderResult{Content: converted, Converted: count, Encoding: "json"}
	}
	if looksLikeClashYAML(trimmed) {
		if converted, count, ok := convertYAML(trimmed, preferredDomain); ok {
			return RenderResult{Content: converted, Converted: count, Encoding: "yaml"}
		}
	}
	return convertLines(raw, preferredDomain)
}

func convertLines(raw, preferredDomain string) RenderResult {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	converted := 0
	for i, line := range lines {
		next, ok := rewriteNodeLine(strings.TrimSpace(line), preferredDomain)
		if ok {
			lines[i] = next
			converted++
		}
	}
	return RenderResult{Content: strings.Join(lines, "\n"), Converted: converted, Encoding: "plain"}
}

func rewriteNodeLine(line, preferredDomain string) (string, bool) {
	if line == "" || strings.HasPrefix(line, "#") {
		return line, false
	}
	lower := strings.ToLower(line)
	if strings.HasPrefix(lower, "vmess://") {
		return rewriteVMess(line, preferredDomain)
	}
	switch {
	case strings.HasPrefix(lower, "vless://"),
		strings.HasPrefix(lower, "trojan://"),
		strings.HasPrefix(lower, "ss://"),
		strings.HasPrefix(lower, "hy2://"),
		strings.HasPrefix(lower, "hysteria2://"),
		strings.HasPrefix(lower, "hysteria://"),
		strings.HasPrefix(lower, "tuic://"),
		strings.HasPrefix(lower, "naive://"):
		return rewriteURLHost(line, preferredDomain)
	default:
		return line, false
	}
}

func rewriteURLHost(line, preferredDomain string) (string, bool) {
	u, err := url.Parse(line)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return line, false
	}
	if port := u.Port(); port != "" {
		u.Host = net.JoinHostPort(preferredDomain, port)
	} else {
		u.Host = preferredDomain
	}
	return u.String(), true
}

func rewriteVMess(line, preferredDomain string) (string, bool) {
	payload := strings.TrimSpace(line[len("vmess://"):])
	decoded, ok := decodeAnyBase64(payload)
	if !ok {
		return line, false
	}
	var node map[string]any
	if err := json.Unmarshal([]byte(decoded), &node); err != nil {
		return line, false
	}
	changed := false
	for _, key := range []string{"add", "server", "address"} {
		if value, ok := node[key].(string); ok && strings.TrimSpace(value) != "" {
			node[key] = preferredDomain
			changed = true
			break
		}
	}
	if !changed {
		return line, false
	}
	b, err := json.Marshal(node)
	if err != nil {
		return line, false
	}
	return "vmess://" + base64.StdEncoding.EncodeToString(b), true
}

func convertJSON(raw, preferredDomain string) (string, int, bool) {
	if raw == "" || (raw[0] != '{' && raw[0] != '[') {
		return "", 0, false
	}
	var data any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return "", 0, false
	}
	count := rewriteStructured(data, preferredDomain)
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", 0, false
	}
	return string(b), count, true
}

func convertYAML(raw, preferredDomain string) (string, int, bool) {
	var data any
	if err := yaml.Unmarshal([]byte(raw), &data); err != nil {
		return "", 0, false
	}
	count := rewriteStructured(data, preferredDomain)
	b, err := yaml.Marshal(data)
	if err != nil {
		return "", 0, false
	}
	return string(b), count, true
}

func rewriteStructured(v any, preferredDomain string) int {
	switch node := v.(type) {
	case map[string]any:
		count := 0
		if looksLikeProxyMap(node) {
			for _, key := range []string{"server", "address", "add"} {
				if value, ok := node[key].(string); ok && strings.TrimSpace(value) != "" {
					node[key] = preferredDomain
					count++
				}
			}
		}
		for _, child := range node {
			count += rewriteStructured(child, preferredDomain)
		}
		return count
	case []any:
		count := 0
		for _, item := range node {
			count += rewriteStructured(item, preferredDomain)
		}
		return count
	default:
		return 0
	}
}

func looksLikeProxyMap(m map[string]any) bool {
	hasAddress := false
	for _, key := range []string{"server", "address", "add"} {
		if _, ok := m[key]; ok {
			hasAddress = true
			break
		}
	}
	if !hasAddress {
		return false
	}
	for _, key := range []string{"type", "port", "server_port", "uuid", "password", "cipher", "name", "tag"} {
		if _, ok := m[key]; ok {
			return true
		}
	}
	return false
}

func looksLikeClashYAML(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.Contains(lower, "proxies:") ||
		strings.Contains(lower, "proxy-groups:") ||
		strings.Contains(lower, "proxy-providers:")
}

func decodeBase64Subscription(raw string) (string, bool) {
	decoded, ok := decodeAnyBase64(raw)
	if !ok || !looksLikeSubscriptionText(decoded) {
		return "", false
	}
	return decoded, true
}

func decodeAnyBase64(raw string) (string, bool) {
	compact := compactBase64(raw)
	if len(compact) < 8 {
		return "", false
	}
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	for _, enc := range encodings {
		b, err := enc.DecodeString(compact)
		if err == nil && utf8.Valid(b) {
			return string(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))), true
		}
	}
	return "", false
}

func compactBase64(raw string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(raw) {
		if !unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func looksLikeSubscriptionText(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "://") ||
		strings.Contains(lower, "proxies:") ||
		strings.Contains(lower, `"outbounds"`) ||
		strings.Contains(lower, "server:")
}
