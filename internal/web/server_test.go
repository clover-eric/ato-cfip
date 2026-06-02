package web

import (
	"strings"
	"testing"
)

func TestPolishAdminHTMLAddsLocalCSVBindingUI(t *testing.T) {
	html := polishAdminHTML(adminHTML)
	for _, want := range []string{"bindTarget", "CSV", "c.csv_url"} {
		if !strings.Contains(html, want) {
			t.Fatalf("polished admin html should contain %q", want)
		}
	}
	if strings.Contains(html, "已发布 IP 数量") {
		t.Fatalf("cloudflare binding success text should describe panel target, not DNS IP publishing")
	}
}

func TestPolishPublicHTMLMentionsCSVSource(t *testing.T) {
	html := polishPublicHTML(publicHTML)
	if !strings.Contains(html, "/best.csv") {
		t.Fatalf("public html should mention the CSV result source")
	}
}
