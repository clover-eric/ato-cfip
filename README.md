# ato-cfip

`ato-cfip` 是一个适合 NAS / 软路由 / 迷你主机 24 小时运行的 Cloudflare/CDN 优选 IP 自动测速服务。

它会定时测速，筛选唯一的优选 IP 池，并提供一个 Web 仪表盘查看当前结果。

## Docker Compose 部署

推荐直接使用仓库根目录唯一的 `docker-compose.yml`：

```yaml
services:
  ato-cfip:
    image: ghcr.io/clover-eric/ato-cfip:latest
    container_name: ato-cfip
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ato-cfip-data:/app/data

volumes:
  ato-cfip-data:
```

启动：

```bash
docker compose up -d
```

访问：

```text
http://NAS_IP:8080
```

## 配置

镜像内置默认配置，首次启动即可运行。默认配置文件在容器内：

```text
/app/config.yaml
```

数据保存在：

```text
/app/data
```

如果要自定义配置，可以把自己的配置挂载进去：

```yaml
services:
  ato-cfip:
    image: ghcr.io/clover-eric/ato-cfip:latest
    container_name: ato-cfip
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ./config.yaml:/app/config.yaml
      - ./data:/app/data
```

## Cloudflare DNS 发布

把配置中的 `publish` 改成：

```yaml
publish:
  mode: "cloudflare-dns"
  domain: "best.example.com"
  ttl: 60
  output_file: "data/best_ips.json"
  hosts_file: "data/hosts.txt"
  cloudflare:
    api_token: "你的 Cloudflare API Token"
    zone_id: "你的 Zone ID"
    proxied: false
```

Cloudflare API Token 建议最小权限：

- Zone: DNS: Edit
- Zone: Zone: Read
- 只授权目标域名所在 Zone

## 域名访问 Web 页面

有公网 IP：

1. 域名 A 记录指向家里公网 IP。
2. 路由器把外部端口转发到 NAS 的 `8080`。
3. 浏览器访问 `http://你的域名:8080`。

没有公网 IP：

使用 Cloudflare Tunnel / FRP / Tailscale Funnel，把域名转发到：

```text
http://127.0.0.1:8080
```

## 注意

- 默认测速地址 `https://cf.xiu2.xyz/url` 不保证长期稳定，正式使用建议自建下载测速 URL。
- 如果 NAS 或路由器走代理，测速结果可能不准确。
- 如果 NAS 无法拉取 `ghcr.io/clover-eric/ato-cfip:latest`，需要先把 GitHub Package 设置为 Public，或改用 Docker Hub / 阿里云 ACR 镜像。

