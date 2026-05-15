# Qoder2API

[![GitHub tag](https://img.shields.io/github/v/tag/wangtufly/qoder2API?include_prereleases)](https://github.com/wangtufly/qoder2API/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**鸣谢：**
- [qoder2api](https://github.com/cubk1/qoder2api)：基于该项目的改造和扩展
- [qodercli-reverse](https://github.com/alingse/qodercli-reverse)：参考了其 Qoder API 接口实现

将 [Qoder](https://qoder.ai) 账号转换为本地 OpenAI / Claude / Codex 兼容 API，供 Cursor、ChatGPT 等客户端直接使用。

## 功能

- 多账号管理，支持 PAT 和 OAuth 登录
- 本地 Bridge 服务，兼容 OpenAI、Claude、Codex API 格式
- 模型映射配置，自定义客户端模型名到 Qoder 模型的映射
- 一键生成客户端配置文件（Cursor、Continue 等）
- 实时日志查看

## 安装

### macOS

从 [Releases](../../releases) 下载对应架构的 `.dmg` 文件：

- `Qoder2API-*-darwin-arm64.dmg` — Apple Silicon (M1/M2/M3)
- `Qoder2API-*-darwin-amd64.dmg` — Intel

打开 DMG，将 `Qoder2API.app` 拖入 Applications 文件夹。

#### 绕过 macOS 安全限制（Gatekeeper）

由于应用未经 Apple 公证，首次打开会被 Gatekeeper 拦截。有两种方式解除：

**方式一：右键打开（推荐）**

1. 在 Finder 中找到 `Qoder2API.app`
2. 按住 `Control` 键单击（或右键单击）
3. 选择「打开」
4. 在弹出的对话框中再次点击「打开」

之后每次可以直接双击启动。

**方式二：命令行移除隔离属性**

```bash
xattr -dr com.apple.quarantine /Applications/Qoder2API.app
```

执行后直接双击即可启动，无需再确认。

### Linux

从 Releases 下载 `qoder2api-*-linux-amd64.tar.gz`，解压后直接运行：

```bash
tar -xzf qoder2api-*-linux-amd64.tar.gz
chmod +x qoder2api
./qoder2api
```

## 使用

1. 启动应用后，在「账号」页面添加 Qoder 账号（PAT 或 OAuth）
2. 设置激活账号，Bridge 服务自动启动（默认端口 `8963`）
3. 在「客户端」页面选择对应工具，点击「一键配置」或手动复制配置

### API 地址

```
http://127.0.0.1:8963
```

### 支持的 API 端点

| 端点 | 兼容格式 |
|------|---------|
| `/v1/chat/completions` | OpenAI Chat |
| `/v1/messages` | Anthropic Claude |
| `/v1/responses` | OpenAI Responses (Codex) |

### Cursor 配置示例

在 Cursor 设置 → Models → OpenAI API Key 填入 Bridge Token（默认 `qoder2api`），Base URL 填入：

```
http://127.0.0.1:8963
```

## 从源码构建

依赖：Go 1.23+、Node.js 18+、[Wails v2](https://wails.io)

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
cd frontend && npm ci && cd ..
wails build
```
