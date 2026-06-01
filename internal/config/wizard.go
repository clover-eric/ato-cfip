package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func InitWizard(path string) error {
	reader := bufio.NewReader(os.Stdin)
	cfg := Defaults()

	fmt.Println("cfst-daemon 初始化向导")
	fmt.Println("留空会使用括号中的默认值。")

	cfg.Publish.Domain = ask(reader, "优选 IP 域名", cfg.Publish.Domain)
	cfg.Web.Title = ask(reader, "Web 页面标题", cfg.Web.Title)
	cfg.Web.Listen = ask(reader, "Web 监听地址", cfg.Web.Listen)
	mode := ask(reader, "发布方式 file/cloudflare-dns", cfg.Publish.Mode)
	cfg.Publish.Mode = mode
	if strings.EqualFold(mode, "cloudflare-dns") || strings.EqualFold(mode, "cloudflare") {
		cfg.Publish.Mode = "cloudflare-dns"
		cfg.Publish.Cloudflare.APIToken = ask(reader, "Cloudflare API Token", cfg.Publish.Cloudflare.APIToken)
		cfg.Publish.Cloudflare.ZoneID = ask(reader, "Cloudflare Zone ID", cfg.Publish.Cloudflare.ZoneID)
		cfg.Publish.Cloudflare.Proxied = false
	}
	cfg.Test.MinSpeedMB = 0.01

	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		backup := path + ".bak"
		if err := os.Rename(path, backup); err != nil {
			return err
		}
		fmt.Printf("已有配置已备份到 %s\n", backup)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return err
	}
	fmt.Printf("配置已写入 %s\n", path)
	return nil
}

func ask(reader *bufio.Reader, label, fallback string) string {
	fmt.Printf("%s (%s): ", label, fallback)
	text, _ := reader.ReadString('\n')
	text = strings.TrimSpace(text)
	if text == "" {
		return fallback
	}
	return text
}
