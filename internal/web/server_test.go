package web

import (
	"strings"
	"testing"
)

func TestPolishAdminHTMLAddsLocalCSVBindingUI(t *testing.T) {
	html := polishAdminHTML(adminHTML)
	for _, want := range []string{"bindPanelURL", "bindTarget", "panel_url", "订阅链接", "CSV", "c.csv_url"} {
		if !strings.Contains(html, want) {
			t.Fatalf("polished admin html should contain %q", want)
		}
	}
	if strings.Contains(html, "已发布 IP 数量") {
		t.Fatalf("cloudflare binding success text should describe panel target, not DNS IP publishing")
	}
}

func TestNormalizePublicBaseURL(t *testing.T) {
	got, err := normalizePublicBaseURL("ip.i3.pub:33668/admin", "http")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://ip.i3.pub:33668" {
		t.Fatalf("unexpected panel url: %s", got)
	}
	got, err = normalizePublicBaseURL("https://ip.i3.pub/best.csv?x=1", "http")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://ip.i3.pub" {
		t.Fatalf("unexpected normalized https panel url: %s", got)
	}
}

func TestPolishPublicHTMLMentionsCSVSource(t *testing.T) {
	html := polishPublicHTML(publicHTML)
	if !strings.Contains(html, "/best.csv") {
		t.Fatalf("public html should mention the CSV result source")
	}
}
