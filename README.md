# ato-cfip

`ato-cfip` 是一个适合 NAS、软路由、迷你主机 24 小时运行的 Cloudflare/CDN 优选 IP 自动测速服务。

## Docker Compose 部署

仓库根目录只保留一份 `docker-compose.yml`。启动后直接访问：

```text
http://NAS_IP:33668
```

首次打开会进入独立初始化页面，设置管理员账户和密码后自动进入：

```text
http://NAS_IP:33668/admin
```

## 页面入口

- `/`：公开状态页，只显示运行状态、归档数量和发布数量，不直接列出 IP。
- `/admin`：管理后台，可查看优选 IP、手动测速、绑定域名、生成订阅、修改管理员账户。
- `/best.csv`：当前优选 IP 池的 CSV 结果源。
- `/ips.csv`：`/best.csv` 的兼容别名。
- `/sub/{token}`：由后台生成的订阅链接。

## 同域名优选模式

新版默认不再把测试结果直接发布成 Cloudflare DNS 记录，而是保存在本机：

```text
/app/data/best_ips.csv
/app/data/best_ips.json
/app/data/hosts.txt
```

这样可以把自己的域名直接绑定到面板程序：

```text
https://你的域名/        公开状态页
https://你的域名/admin   管理后台
https://你的域名/best.csv 优选 CSV 结果源
```

后台的“绑定域名”会引导用户通过 Cloudflare API 把域名指向 NAS 面板入口。目标地址可以留空自动检测公网 IP，也可以手动填写公网 IP 或 CNAME 目标。

说明：同一个裸域名在 DNS 层不能同时解析到 NAS 和 10 个 Cloudflare 优选 IP。现在的做法是让域名指向面板程序，再由程序提供 CSV 和订阅转换。订阅生成时会优先读取当前优选 IP 池，把原节点展开成优选 IP 节点，同时保留原节点的 SNI、Host、路径、密钥和端口。

## 国内 GitHub 加速安装

国内 NAS 拉取 GitHub 慢时，默认使用：

```text
https://github.i3.pub
```

Linux / 群晖 SSH：

```bash
curl -fsSL https://github.i3.pub/https://raw.githubusercontent.com/clover-eric/ato-cfip/main/scripts/install.sh | sh
```

手动指定加速器：

```bash
ATO_CFIP_GITHUB_ACCELERATOR=https://github.i3.pub sh scripts/install.sh
```

Windows PowerShell：

```powershell
iwr https://github.i3.pub/https://raw.githubusercontent.com/clover-eric/ato-cfip/main/scripts/install.ps1 -UseBasicParsing | iex
```

不使用加速器：

```bash
ATO_CFIP_REPO=https://github.com/clover-eric/ato-cfip.git sh scripts/install.sh
```

## 数据目录

容器内数据保存在：

```text
/app/data
```

Docker Compose 默认使用命名卷：

```text
ato-cfip-data
```
