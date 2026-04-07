# DS2API 部署指南

语言 / Language: [中文](DEPLOY.md) | [English](DEPLOY.en.md)

本指南基于当前 Go 代码库，详细说明各种部署方式。

本页导航：[文档总索引](./README.md)｜[架构说明](./ARCHITECTURE.md)｜[接口文档](../API.md)｜[测试指南](./TESTING.md)

---

## 目录

- [前置要求](#0-前置要求)
- [一、本地运行](#一本地运行)
- [二、Docker 部署](#二docker-部署)
- [三、Vercel 部署](#三vercel-部署)
- [四、下载 Release 构建包](#四下载-release-构建包)
- [五、反向代理（Nginx）](#五反向代理nginx)
- [六、Linux systemd 服务化](#六linux-systemd-服务化)
- [七、部署后检查](#七部署后检查)
- [八、发布前进行本地回归](#八发布前进行本地回归)

---

## 0. 前置要求

| 依赖 | 最低版本 | 说明 |
| --- | --- | --- |
| Go | 1.26+ | 编译后端 |
| Node.js | `20.19+` 或 `22.12+` | 仅在需要本地构建 WebUI 时 |
| npm | 随 Node.js 提供 | 安装 WebUI 依赖 |

配置来源（任选其一）：

- **文件方式**：`config.json`（推荐本地/Docker 使用）
- **环境变量方式**：`DS2API_CONFIG_JSON`（推荐 Vercel 使用，支持 JSON 字符串或 Base64 编码，也可以直接写原始 JSON）

统一建议（最优实践）：

```bash
cp config.example.json config.json
# 编辑 config.json
```

建议把 `config.json` 作为唯一配置源：
- 本地运行：直接读 `config.json`
- Docker / Vercel：从 `config.json` 生成 `DS2API_CONFIG_JSON`（Base64）注入环境变量

---

## 一、本地运行

### 1.1 基本步骤

```bash
# 克隆仓库
git clone https://github.com/CJackHwang/ds2api.git
cd ds2api

# 复制并编辑配置
cp config.example.json config.json
# 使用你喜欢的编辑器打开 config.json，填入：
#   - keys: 你的 API 访问密钥
#   - accounts: DeepSeek 账号（email 或 mobile + password）

# 启动服务
go run ./cmd/ds2api
```

默认本地访问地址是 `http://127.0.0.1:5001`；服务实际绑定 `0.0.0.0:5001`，可通过 `PORT` 环境变量覆盖。

### 1.2 WebUI 构建

本地首次启动时，若 `static/admin/` 不存在，服务会自动尝试构建 WebUI（需要 Node.js/npm；缺依赖时会先执行 `npm ci`，再执行 `npm run build -- --outDir static/admin --emptyOutDir`）。

你也可以手动构建：

```bash
./scripts/build-webui.sh
```

或手动执行：

```bash
cd webui
npm install
npm run build
# 产物输出到 static/admin/
```

通过环境变量控制自动构建行为：

```bash
# 强制关闭自动构建
DS2API_AUTO_BUILD_WEBUI=false go run ./cmd/ds2api

# 强制开启自动构建
DS2API_AUTO_BUILD_WEBUI=true go run ./cmd/ds2api
```

### 1.3 编译为二进制文件

```bash
go build -o ds2api ./cmd/ds2api
./ds2api
```

---

## 二、Docker 部署

### 2.1 基本步骤

```bash
# 复制环境变量模板和配置文件
cp .env.example .env
cp config.example.json config.json

# 编辑 .env（请改成你的强密码），至少设置：
#   DS2API_ADMIN_KEY=your-admin-key
# 如需修改宿主机端口，可额外设置：
#   DS2API_HOST_PORT=6011

# 启动
docker-compose up -d

# 查看日志
docker-compose logs -f
```

默认 `docker-compose.yml` 会把宿主机 `6011` 映射到容器内的 `5001`。如果你希望直接对外暴露 `5001`，请设置 `DS2API_HOST_PORT=5001`（或者手动调整 `ports` 配置）。

### 2.2 更新

```bash
docker-compose up -d --build
```

### 2.3 Docker 架构说明

`Dockerfile` 提供两条构建路径：

1. **本地/开发默认路径（`runtime-from-source`）**：三阶段构建（WebUI 构建 + Go 构建 + 运行阶段）。
2. **Release 路径（`runtime-from-dist`）**：发布工作流先生成 tag 命名的 Release 压缩包，再把 Linux 产物复制成 `dist/docker-input/linux_amd64.tar.gz` / `linux_arm64.tar.gz`；Docker 构建阶段直接消费这些输入，不再重复执行 `npm build`/`go build`。

Release 路径可确保 Docker 镜像与 release 压缩包使用同一套产物，减少重复构建带来的差异。

容器内启动命令：`/usr/local/bin/ds2api`，默认暴露端口 `5001`。

### 2.4 开发环境

```bash
docker-compose -f docker-compose.dev.yml up
```

开发模式特性：
- 源代码挂载（修改即生效）
- `LOG_LEVEL=DEBUG`
- 不自动重启

### 2.5 健康检查

Docker Compose 已配置内置健康检查：

```yaml
healthcheck:
  test: ["CMD", "/usr/local/bin/busybox", "wget", "-qO-", "http://localhost:${PORT:-5001}/healthz"]
  interval: 30s
  timeout: 10s
  retries: 3
  start_period: 10s
```

### 2.6 Docker 常见排查

如果容器日志正常但面板打不开，优先检查：

1. **端口是否一致**：`PORT` 改成非 `5001` 时，访问地址也要改成对应端口（如 `http://localhost:8080/admin`）。
2. **开发 compose 的 WebUI 静态文件**：`docker-compose.dev.yml` 使用 `go run` 开发镜像，不会在容器内自动安装 Node.js；若仓库里没有 `static/admin`，`/admin` 会返回 404。可先在宿主机构建一次：`./scripts/build-webui.sh`。

### 2.7 Zeabur 一键部署（Dockerfile）

仓库提供 `zeabur.yaml` 模板，可在 Zeabur 上一键部署：

[![Deploy on Zeabur](https://zeabur.com/button.svg)](https://zeabur.com/templates/L4CFHP)

部署要点：

- **端口**：服务默认监听 `5001`，模板会固定设置 `PORT=5001`。
- **配置持久化**：模板挂载卷 `/data`，并设置 `DS2API_CONFIG_PATH=/data/config.json`；在管理台导入配置后，会写入并持久化到该路径。
- **构建版本号**：Zeabur / 普通 `docker build` 默认不需要传 `BUILD_VERSION`；镜像会优先使用该构建参数，未提供时自动回退到仓库根目录的 `VERSION` 文件。
- **首次登录**：部署完成后访问 `/admin`，使用 Zeabur 环境变量/模板指引中的 `DS2API_ADMIN_KEY` 登录（建议首次登录后自行更换为强密码）。

---

## 三、Vercel 部署

### 3.1 部署步骤

1. **Fork 仓库**到你的 GitHub 账号
2. **在 Vercel 上导入项目**
3. **配置环境变量**（最少只需设置以下一项）：

| 变量 | 说明 |
| --- | --- |
| `DS2API_ADMIN_KEY` | 管理密钥（必填） |
| `DS2API_CONFIG_JSON` | 配置内容，JSON 字符串或 Base64 编码（可选，建议） |

4. **部署**

### 3.1.1 推荐填写方式（避免 `DS2API_CONFIG_JSON` 填错）

如果你想先完成一键部署，也可以先不填 `DS2API_CONFIG_JSON`，部署后进入 `/admin` 导入配置，再在「Vercel 同步」里写回环境变量。

建议先在仓库目录复制示例配置，再按实际账号填写：

```bash
cp config.example.json config.json
# 编辑 config.json
```

不要在 Vercel 面板里手写复杂 JSON，建议本地生成 Base64 后粘贴：

```bash
# 在仓库根目录执行
DS2API_CONFIG_JSON="$(base64 < config.json | tr -d '\n')"
echo "$DS2API_CONFIG_JSON"
```

如果你选择在部署前就预置配置，请在 Vercel Project Settings -> Environment Variables 配置：

```text
DS2API_ADMIN_KEY=请替换为强密码
DS2API_CONFIG_JSON=上一步生成的一整行 Base64
```

可选但推荐（用于 WebUI 一键同步 Vercel 配置）：

```text
VERCEL_TOKEN=你的 Vercel Token
VERCEL_PROJECT_ID=prj_xxxxxxxxxxxx
VERCEL_TEAM_ID=team_xxxxxxxxxxxx   # 个人账号可留空
```

### 3.2 可选环境变量

| 变量 | 说明 | 默认值 |
| --- | --- | --- |
| `DS2API_ACCOUNT_MAX_INFLIGHT` | 每账号并发上限 | `2` |
| `DS2API_ACCOUNT_MAX_QUEUE` | 等待队列上限 | `recommended_concurrency` |
| `DS2API_GLOBAL_MAX_INFLIGHT` | 全局并发上限 | `recommended_concurrency` |
| `DS2API_ENV_WRITEBACK` | 检测到 `DS2API_CONFIG_JSON` 时自动写入 `DS2API_CONFIG_PATH`，并在成功后转为文件模式（`1/true/yes/on`） | 关闭 |
| `DS2API_VERCEL_INTERNAL_SECRET` | 混合流式内部鉴权 | 回退用 `DS2API_ADMIN_KEY` |
| `DS2API_VERCEL_STREAM_LEASE_TTL_SECONDS` | 流式 lease TTL | `900` |
| `VERCEL_TOKEN` | Vercel 同步 token | — |
| `VERCEL_PROJECT_ID` | Vercel 项目 ID | — |
| `VERCEL_TEAM_ID` | Vercel 团队 ID | — |
| `DS2API_VERCEL_PROTECTION_BYPASS` | 部署保护绕过密钥（内部 Node→Go 调用） | — |

### 3.3 Vercel 架构说明

```text
请求 ─────┐
          │
          ▼
     vercel.json 路由规则
          │
    ┌─────┴─────┐
    │           │
    ▼           ▼
api/index.go  api/chat-stream.js
(Go Runtime)  (Node Runtime)
```

- **入口文件**：`api/index.go`（Serverless Go）
- **流式入口**：`api/chat-stream.js`（Node Runtime，保证实时 SSE）
- **路由重写**：`vercel.json`
- **构建命令**：`npm ci --prefix webui && npm run build --prefix webui`（自动执行）

#### 流式处理链路

由于 Vercel Go Runtime 存在平台层响应缓冲，本项目在 Vercel 上采用"**Go prepare + Node stream**"的混合链路：

1. `api/chat-stream.js` 收到 `/v1/chat/completions` 请求
2. Node 调用 Go 内部 prepare 接口（`?__stream_prepare=1`），获取会话 ID、PoW、token 等
3. Go prepare 创建 stream lease，锁定账号
4. Node 直连 DeepSeek 上游，实时流式转发 SSE 给客户端（含 OpenAI chunk 封装与 tools 防泄漏筛分）
5. 流结束后 Node 调用 Go release 接口（`?__stream_release=1`），释放账号

> 该适配**仅在 Vercel 环境生效**；本地与 Docker 仍走纯 Go 链路。

#### 非流式回退与 Tool Call 处理

- `api/chat-stream.js` 仅对非流式请求回退到 Go 入口（`?__go=1`）
- 流式请求（包括带 `tools`）走 Node 路径，并执行与 Go 对齐的 tool-call 防泄漏处理
- WebUI 的"非流式测试"直接请求 `?__go=1`，避免 Node 中转造成长请求超时

#### 函数时长

`vercel.json` 已将 `api/chat-stream.js` 与 `api/index.go` 的 `maxDuration` 设为 `300`（受 Vercel 套餐上限约束）。

### 3.4 Vercel 常见报错排查

#### Go 构建失败

```text
Error: Command failed: go build -ldflags -s -w -o .../bootstrap ...
```

**原因**：Vercel 项目的 Go 构建参数配置不正确（`-ldflags` 没有作为一个整体字符串传递）。

**解决**：

1. 进入 Vercel Project Settings → Build and Development Settings
2. **清空**自定义 Go Build Flags / Build Command（推荐）
3. 若必须设置 ldflags，使用 `-ldflags="-s -w"`（保证它是一个参数）
4. 确认仓库 `go.mod` 为受支持版本（当前为 `go 1.26.0`）
5. 重新部署（建议清缓存后 Redeploy）

#### Internal 包导入错误

```text
use of internal package ds2api/internal/server not allowed
```

**原因**：Vercel Go 入口文件直接 `import internal/...`。

**解决**：当前仓库已通过公开桥接包 `app` 解决：`api/index.go` → `ds2api/app` → `internal/server`。

#### 输出目录错误

```text
No Output Directory named "public" found after the Build completed.
```

**解决**：当前仓库使用 `static` 作为输出目录（`vercel.json` 中 `"outputDirectory": "static"`）。若你在项目设置里手动改过 Output Directory，请设为 `static` 或清空让仓库配置生效。

#### 部署保护拦截

如果接口返回 Vercel HTML 页面 `Authentication Required`：

- **方案 A**：关闭该部署/环境的 Deployment Protection（推荐用于公开 API）
- **方案 B**：请求中添加 `x-vercel-protection-bypass` 头
- **方案 C**：设置 `VERCEL_AUTOMATION_BYPASS_SECRET`（或 `DS2API_VERCEL_PROTECTION_BYPASS`），仅影响内部 Node→Go 调用

### 3.5 仓库不提交构建产物

- `static/admin` 目录不在 Git 中
- Vercel / Docker 构建阶段自动生成 WebUI 静态文件

---

## 四、下载 Release 构建包

仓库内置 GitHub Actions 工作流：`.github/workflows/release-artifacts.yml`

- **触发条件**：仅在 Release `published` 时触发（普通 push 不会构建）
- **构建产物**：多平台二进制压缩包 + `sha256sums.txt`
- **容器镜像发布**：仅发布到 GHCR（`ghcr.io/cjackhwang/ds2api`）

| 平台 | 架构 | 文件格式 |
| --- | --- | --- |
| Linux | amd64, arm64 | `.tar.gz` |
| macOS | amd64, arm64 | `.tar.gz` |
| Windows | amd64 | `.zip` |

每个压缩包包含：

- `ds2api` 可执行文件（Windows 为 `ds2api.exe`）
- `static/admin/`（WebUI 构建产物）
- `config.example.json`、`.env.example`
- `README.MD`、`README.en.md`、`LICENSE`

### 使用步骤

```bash
# 1. 下载对应平台的压缩包
# 2. 解压
tar -xzf ds2api_<tag>_linux_amd64.tar.gz
cd ds2api_<tag>_linux_amd64

# 3. 配置
cp config.example.json config.json
# 编辑 config.json

# 4. 启动
./ds2api
```

### 维护者发布步骤

1. 在 GitHub 创建并发布 Release（带 tag，如 `vX.Y.Z`）
2. 等待 Actions 工作流 `Release Artifacts` 完成
3. 在 Release 的 Assets 下载对应平台压缩包

### 拉取 GHCR 镜像（可选）

```bash
# latest
docker pull ghcr.io/cjackhwang/ds2api:latest

# 指定版本（示例）
docker pull ghcr.io/cjackhwang/ds2api:v3.0.0
```

---

## 五、反向代理（Nginx）

如果在 Nginx 后部署，**必须关闭缓冲**以保证 SSE 流式响应正常工作：

```nginx
location / {
    proxy_pass http://127.0.0.1:5001;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_buffering off;
    proxy_cache off;
    chunked_transfer_encoding on;
    tcp_nodelay on;
}
```

如果需要 HTTPS，可以在 Nginx 层配置 SSL 证书：

```nginx
server {
    listen 443 ssl;
    server_name api.example.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://127.0.0.1:5001;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffering off;
        proxy_cache off;
        chunked_transfer_encoding on;
        tcp_nodelay on;
    }
}
```

---

## 六、Linux systemd 服务化

### 6.1 安装

```bash
# 将编译好的二进制文件和相关文件复制到目标目录
sudo mkdir -p /opt/ds2api
sudo cp ds2api config.json /opt/ds2api/
sudo cp -r static/admin /opt/ds2api/static/admin
```

### 6.2 创建 systemd 服务文件

```ini
# /etc/systemd/system/ds2api.service

[Unit]
Description=DS2API (Go)
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/ds2api
Environment=PORT=5001
Environment=DS2API_CONFIG_PATH=/opt/ds2api/config.json
Environment=DS2API_ADMIN_KEY=your-admin-key-here
ExecStart=/opt/ds2api/ds2api
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### 6.3 常用命令

```bash
# 加载服务配置
sudo systemctl daemon-reload

# 设置开机自启
sudo systemctl enable ds2api

# 启动服务
sudo systemctl start ds2api

# 查看状态
sudo systemctl status ds2api

# 查看日志
sudo journalctl -u ds2api -f

# 重启服务
sudo systemctl restart ds2api

# 停止服务
sudo systemctl stop ds2api
```

---

## 七、部署后检查

无论使用哪种部署方式，启动后建议依次检查：

```bash
# 1. 存活探针
curl -s http://127.0.0.1:5001/healthz
# 预期: {"status":"ok"}

# 2. 就绪探针
curl -s http://127.0.0.1:5001/readyz
# 预期: {"status":"ready"}

# 3. 模型列表
curl -s http://127.0.0.1:5001/v1/models
# 预期: {"object":"list","data":[...]}

# 4. 管理台页面（如果已构建 WebUI）
curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:5001/admin
# 预期: 200

# 5. 测试 API 调用
curl http://127.0.0.1:5001/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"model":"deepseek-chat","messages":[{"role":"user","content":"hello"}]}'
```

---

## 八、发布前进行本地回归

建议在发布前执行完整的端到端测试集（使用真实账号）：

```bash
./tests/scripts/run-live.sh
```

可自定义参数：

```bash
go run ./cmd/ds2api-tests \
  --config config.json \
  --admin-key admin \
  --out artifacts/testsuite \
  --timeout 120 \
  --retries 2
```

测试集自动执行内容：

- ✅ 语法/构建/单测 preflight
- ✅ 隔离副本配置启动服务（不污染原始 `config.json`）
- ✅ 真实调用场景验证（OpenAI/Claude/Admin/并发/toolcall/流式）
- ✅ 全量请求与响应日志落盘（用于故障复盘）

详细测试集说明参阅 [TESTING.md](TESTING.md)。
