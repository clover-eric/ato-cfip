# cfst-daemon

`cfst-daemon` 是一个适合 NAS / 软路由 / 迷你主机 24 小时运行的 Cloudflare/CDN 优选 IP 自动测速服务。

它做三件事：

1. 每小时自动测速多轮，筛出唯一的优选 IP 池。
2. 自动发布到文件、hosts 或 Cloudflare DNS。
3. 内置 Web 仪表盘，打开你绑定的域名即可看到当前优选结果和运行状态。

## 最简单部署

### 单文件 Compose 部署

如果你的 NAS 拉不动 Docker Hub/GHCR 镜像，优先使用这个文件：

```bash
curl -fsSL -o docker-compose.yml https://raw.githubusercontent.com/clover-eric/ato-cfip/main/docker-compose.standalone.yml
docker compose up -d --build
```

这个版本不拉 GHCR 镜像，也不拉 golang/alpine 镜像；它会在构建时直接下载仓库里的静态二进制和默认配置文件。

如果 NAS 面板不支持自动识别架构，可以直接使用固定架构版本：

amd64:

```bash
curl -fsSL -o docker-compose.yml https://raw.githubusercontent.com/clover-eric/ato-cfip/main/docker-compose.amd64.yml
docker compose up -d --build
```

arm64:

```bash
curl -fsSL -o docker-compose.yml https://raw.githubusercontent.com/clover-eric/ato-cfip/main/docker-compose.arm64.yml
docker compose up -d --build
```

Linux / NAS 一键安装：

```bash
curl -fsSL https://raw.githubusercontent.com/clover-eric/ato-cfip/main/scripts/install.sh | sh
```

Windows / PowerShell 一键安装：

```powershell
iwr -useb https://raw.githubusercontent.com/clover-eric/ato-cfip/main/scripts/install.ps1 | iex
```

手动 Docker 部署：

```bash
cp .env.example .env
docker compose up -d
```

然后访问：

```text
http://NAS_IP:8080
```

`docker-compose.yml` 会自动创建配置卷和数据卷，并在第一次启动时生成 `/config/config.yaml`。如果你只想在 NAS 面板里粘贴 compose 文件，也可以直接使用仓库里的 `docker-compose.yml` 内容，不需要额外上传 `config.yaml`。

如果要在首次启动时直接设置域名和 Cloudflare DNS：

```env
CFST_WEB_PORT=8080
CFST_DOMAIN=best.example.com
CFST_PUBLISH_MODE=cloudflare-dns
CFST_CF_API_TOKEN=你的CloudflareToken
CFST_CF_ZONE_ID=你的ZoneID
```

注意：这些环境变量只在第一次生成配置时写入。如果已经启动过，需要进入配置卷修改 `/config/config.yaml`，或删除配置卷后重建。

如果在 Windows 本机测试：

```powershell
.\cfst-daemon.exe -config config.yaml -init
.\cfst-daemon.exe -config config.smoke.yaml -once
.\cfst-daemon.exe -config config.yaml
```

## 绑定自己的域名

有公网 IP：

1. 域名添加 A 记录到家里的公网 IP。
2. 路由器端口转发：外部 `8080` 或 `443` -> NAS `8080`。
3. 浏览器打开 `http://你的域名:8080`。

没有公网 IP：

1. 使用 Cloudflare Tunnel / FRP / Tailscale Funnel。
2. 将你的域名转发到 NAS 的 `http://127.0.0.1:8080`。
3. 外网打开域名即可看到仪表盘。

注意：这个 Web 页面只是让用户查看状态，不影响“优选域名”的 DNS 发布。NAS 不暴露到公网时，程序仍然可以调用 DNS 服务商 API 更新优选 IP 域名。

## 发布优选 IP 到 Cloudflare DNS

把 `config.yaml` 改成：

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

Cloudflare API Token 建议使用最小权限：

- Zone: DNS: Edit
- Zone: Zone: Read
- 只授权目标域名所在的 Zone

Cloudflare DNS 记录必须保持 `proxied: false`，也就是 DNS only。程序会维护该域名下的 A/AAAA 记录。

## 常用配置

```yaml
web:
  enabled: true
  listen: ":8080"
  title: "CFST 优选节点"

server:
  run_on_start: true
  schedule: "0 * * * *"      # 每小时整点，也可写 "@every 1h"
  timezone: "Asia/Shanghai"

test:
  rounds_per_hour: 10
  desired_unique_ips: 10
  max_extra_rounds: 30
  delay_threads: 200
  ping_times: 4
  download_time_seconds: 10
  download_candidates: 10
  min_speed_mb: 0.01
```

当前轻量调度器支持：

- `0 * * * *`：每小时整点执行
- `@every 1h`：每隔 1 小时执行
- `@every 30m`：每隔 30 分钟执行

## 输出文件

- `data/best_ips.json`：当前发布的优选 IP 池
- `data/hosts.txt`：hosts 格式结果
- `data/results.jsonl`：历史记录，一行一个 JSON

## NAS 拉镜像失败怎么办

如果 NAS 无法连接 Docker Hub 或 GHCR，先试试单文件 Compose：

```bash
curl -fsSL -o docker-compose.yml https://raw.githubusercontent.com/clover-eric/ato-cfip/main/docker-compose.standalone.yml
docker compose up -d --build
```

如果 NAS 连 GitHub Release 也无法访问，再在电脑上构建离线镜像并导入 NAS。

amd64 NAS：

```powershell
$env:GOOS="linux"
$env:GOARCH="amd64"
$env:CGO_ENABLED="0"
go build -o ato-cfip ./cmd/cfst-daemon
docker build -f Dockerfile.local -t ato-cfip:local .
docker save ato-cfip:local -o ato-cfip-local.tar
```

arm64 NAS：

```powershell
$env:GOOS="linux"
$env:GOARCH="arm64"
$env:CGO_ENABLED="0"
go build -o ato-cfip ./cmd/cfst-daemon
docker build -f Dockerfile.local -t ato-cfip:local .
docker save ato-cfip:local -o ato-cfip-local.tar
```

把 `ato-cfip-local.tar` 上传到 NAS：

```bash
docker load -i ato-cfip-local.tar
docker compose -f docker-compose.local.yml up -d
```

## 重要建议

- 默认测速地址 `https://cf.xiu2.xyz/url` 不保证长期稳定，正式使用建议自建下载测速 URL。
- 如果运行 NAS 或路由器走了代理，测速结果可能不准确。
- 无公网 IP 不影响程序自动更新 DNS；只有访问 Web 仪表盘才需要公网入口或隧道。
- 先用 `publish.mode: file` 跑通，再切换到 `cloudflare-dns`。
