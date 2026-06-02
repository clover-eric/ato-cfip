package subscription

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestConvertVLESSLine(t *testing.T) {
	got, ok := rewriteNodeLine("vless://uuid@example.com:443?security=tls&sni=origin.example.com&type=ws&host=origin.example.com#node", "best.example.com")
	if !ok {
		t.Fatal("expected vless line to be rewritten")
	}
	if !strings.Contains(got, "uuid@best.example.com:443") {
		t.Fatalf("address was not rewritten: %s", got)
	}
	if !strings.Contains(got, "sni=origin.example.com") || !strings.Contains(got, "host=origin.example.com") {
		t.Fatalf("sni/host should be preserved: %s", got)
	}
}

func TestConvertVMessLine(t *testing.T) {
	raw := map[string]any{"v": "2", "ps": "node", "add": "origin.example.com", "port": "443", "host": "origin.example.com", "sni": "origin.example.com"}
	b, _ := json.Marshal(raw)
	line := "vmess://" + base64.StdEncoding.EncodeToString(b)
	got, ok := rewriteNodeLine(line, "best.example.com")
	if !ok {
		t.Fatal("expected vmess line to be rewritten")
	}
	decoded, ok := decodeAnyBase64(strings.TrimPrefix(got, "vmess://"))
	if !ok {
		t.Fatal("rewritten vmess payload is not base64")
	}
	var node map[string]any
	if err := json.Unmarshal([]byte(decoded), &node); err != nil {
		t.Fatal(err)
	}
	if node["add"] != "best.example.com" {
		t.Fatalf("add was not rewritten: %#v", node["add"])
	}
	if node["host"] != "origin.example.com" || node["sni"] != "origin.example.com" {
		t.Fatalf("host/sni should be preserved: %#v", node)
	}
}

func TestConvertBase64Subscription(t *testing.T) {
	source := "vless://uuid@example.com:443?security=tls&sni=origin.example.com#node"
	encoded := base64.StdEncoding.EncodeToString([]byte(source))
	got, err := Convert(encoded, "best.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.Encoding != "base64" || got.Converted != 1 {
		t.Fatalf("unexpected result: %#v", got)
	}
	decoded, ok := decodeAnyBase64(got.Content)
	if !ok || !strings.Contains(decoded, "uuid@best.example.com:443") {
		t.Fatalf("converted subscription was not encoded correctly: %s", got.Content)
	}
}

func TestConvertWithAddressPoolExpandsPlainNodes(t *testing.T) {
	source := "vless://uuid@example.com:443?security=tls&sni=origin.example.com#node"
	got, err := ConvertWithAddresses(source, []string{"1.1.1.1", "1.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Converted != 2 {
		t.Fatalf("expected two converted nodes, got %#v", got)
	}
	if !strings.Contains(got.Content, "uuid@1.1.1.1:443") || !strings.Contains(got.Content, "uuid@1.0.0.1:443") {
		t.Fatalf("address pool was not expanded: %s", got.Content)
	}
	if !strings.Contains(got.Content, "CFIP%2001") || !strings.Contains(got.Content, "CFIP%2002") {
		t.Fatalf("expanded nodes should be labeled: %s", got.Content)
	}
}

func TestConvertClashYAML(t *testing.T) {
	source := "proxies:\n  - name: cf\n    type: vless\n    server: origin.example.com\n    port: 443\n    sni: origin.example.com\n"
	got, err := Convert(source, "best.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.Encoding != "yaml" || got.Converted != 1 {
		t.Fatalf("unexpected result: %#v", got)
	}
	if !strings.Contains(got.Content, "server: best.example.com") {
		t.Fatalf("server was not rewritten: %s", got.Content)
	}
	if !strings.Contains(got.Content, "sni: origin.example.com") {
		t.Fatalf("sni should be preserved: %s", got.Content)
	}
}
