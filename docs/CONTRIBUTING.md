# 贡献指南

语言 / Language: [中文](CONTRIBUTING.md) | [English](CONTRIBUTING.en.md)

感谢你对 DS2API 的关注与贡献！

## 开发环境设置

### 前置要求

- Go 1.26+
- Node.js `20.19+` 或 `22.12+`（WebUI 开发时）
- npm（随 Node.js 提供）

### 后端开发

```bash
# 1. 克隆仓库
git clone https://github.com/CJackHwang/ds2api.git
cd ds2api

# 2. 配置
cp config.example.json config.json
# 编辑 config.json，填入测试账号

# 3. 启动后端
go run ./cmd/ds2api
# 本地访问 http://127.0.0.1:5001
# 实际绑定 0.0.0.0:5001，可通过局域网 IP 访问
```

### 前端开发（WebUI）

```bash
# 1. 进入 WebUI 目录
cd webui

# 2. 安装依赖
npm install

# 3. 启动开发服务器（热更新）
npm run dev
# 默认监听 http://localhost:5173，自动代理 API 到后端
# 当前未配置 host: 0.0.0.0，因此默认不对局域网开放
```

WebUI 技术栈：
- React + Vite
- Tailwind CSS
- 中英文语言包：`webui/src/locales/zh.json` / `en.json`

### Docker 开发环境

```bash
docker-compose -f docker-compose.dev.yml up
```

## 代码规范

| 语言 | 规范 |
| --- | --- |
| **Go** | 提交前运行 `./scripts/lint.sh`（包含 gofmt+golangci-lint）并确保 `go test ./...` 通过 |
| **JavaScript/React** | 保持现有代码风格（函数组件） |
| **提交信息** | 使用语义化前缀：`feat:`、`fix:`、`docs:`、`refactor:`、`style:`、`perf:`、`chore:` |

## 提交 PR

1. Fork 仓库
2. 创建分支（如 `feature/xxx` 或 `fix/xxx`）
3. 提交更改
4. 推送分支
5. 发起 Pull Request

> 💡 如果修改了 `webui/` 目录下的文件，无需手动构建——CI 会自动处理。
> 但如果你本地想验证 `static/admin/` 产物，还是可以手动运行 `./scripts/build-webui.sh`。

## WebUI 构建

手动构建 WebUI 到 `static/admin/`：

```bash
./scripts/build-webui.sh
```

## 运行测试

```bash
# Go + Node 单元测试（推荐）
./tests/scripts/run-unit-all.sh

# 端到端全链路测试（真实账号）
./tests/scripts/run-live.sh
```

## 项目结构

为避免与其他文档重复维护，目录结构与模块职责已迁移到：

- [docs/ARCHITECTURE.md](./ARCHITECTURE.md)
- [docs/README.md](./README.md)

贡献前建议先阅读架构文档中的“请求主链路”和 `internal/` 模块职责，再定位改动范围。

## 问题反馈

请使用 [GitHub Issues](https://github.com/CJackHwang/ds2api/issues) 并附上：

- 复现步骤
- 相关日志输出
- 运行环境信息（OS、Go 版本、部署方式）
