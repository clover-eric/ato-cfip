# ato-cfip

`ato-cfip` 是一个适合 NAS、软路由、迷你主机 24 小时运行的 Cloudflare/CDN 优选 IP 自动测速服务。

## Docker Compose 部署

仓库根目录只保留一份 `docker-compose.yml`：

```yaml
services:
  ato-cfip:
    image: ghcr.io/clover-eric/ato-cfip:latest
    container_name: ato-cfip
    restart: unless-stopped
    ports:
      - "33668:8080"
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
http://NAS_IP:33668
```

首次打开会进入独立初始化页面，设置管理员账户和密码后自动跳转到：

```text
http://NAS_IP:33668/admin
```

## 国内 GitHub 加速安装

国内 NAS 拉取 GitHub 慢时，默认使用：

```text
https://github.i3.pub
```

Linux / 群晖 SSH：

```bash
curl -fsSL https://github.i3.pub/https://raw.githubusercontent.com/clover-eric/ato-cfip/main/scripts/install.sh | sh
```

也可以手动指定加速器：

```bash
ATO_CFIP_GITHUB_ACCELERATOR=https://github.i3.pub sh scripts/install.sh
```

Windows PowerShell：

```powershell
iwr https://github.i3.pub/https://raw.githubusercontent.com/clover-eric/ato-cfip/main/scripts/install.ps1 -UseBasicParsing | iex
```

如果不想使用加速器，可以覆盖仓库地址：

```bash
ATO_CFIP_REPO=https://github.com/clover-eric/ato-cfip.git sh scripts/install.sh
```

## 页面入口

- `/`：公开状态页，只显示运行状态、已测归档数量，不暴露具体 IP。
- `/admin`：管理后台，可查看优选 IP、手动测速、绑定域名、修改管理员账户。

## 域名说明

面板可以使用 `IP:端口` 或 `域名:端口` 访问，默认端口为 `33668`。

优选域名本身不能带端口。后台的“绑定域名”会引导用户把 DNS A 记录指向自己的公网 IP。没有公网 IP 的用户可以先只在局域网使用面板，后续再接入 Cloudflare Tunnel、FRP 或 Tailscale Funnel。

## 数据目录

容器内数据保存在：

```text
/app/data
```

Docker Compose 默认使用命名卷：

```text
ato-cfip-data
```
