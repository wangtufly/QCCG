# QCCG

[![GitHub Release](https://img.shields.io/github/v/release/wangtufly/qoder2API)](https://github.com/wangtufly/qoder2API/releases)
[![Go](https://img.shields.io/badge/Go-1.25-blue)](https://go.dev)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

将 [Qoder](https://qoder.ai) 账号转换为本地 OpenAI / Claude / Codex 兼容 API，供 Cursor、ChatGPT 等客户端直接使用。

## 功能

- 多账号管理，支持 PAT 和 OAuth 登录
- 本地 Bridge 服务，兼容 OpenAI、Claude、Codex API 格式
- 自定义模型映射配置
- 一键生成客户端配置文件（Cursor、Continue 等）

## 安装

### macOS

从 [Releases](../../releases) 下载 `.dmg` 文件，打开后将 `QCCG.app` 拖入 Applications。

首次打开需绕过 Gatekeeper，两种方式：

```bash
# 方式一：命令行移除隔离
xattr -dr com.apple.quarantine /Applications/QCCG.app

# 方式二：右键 QCCG.app → 打开 → 再次点击「打开」
```

### Linux

```bash
tar -xzf qccg-*-linux-amd64.tar.gz
chmod +x qccg
./qccg
```

## 使用

1. 启动应用，在「账号」页面添加 Qoder 账号（PAT 或 OAuth）
2. 设置激活账号，Bridge 服务自动启动（默认端口 `8963`）
3. 在「客户端」页面选择工具，一键配置或复制配置

### API 地址

```
http://127.0.0.1:8963
```

### 支持的端点

| 端点 | 兼容格式 |
|------|---------|
| `/v1/chat/completions` | OpenAI Chat |
| `/v1/messages` | Anthropic Claude |
| `/v1/responses` | OpenAI Responses (Codex) |

### Cursor 配置

在 Cursor 设置 → Models → OpenAI API Key 填入 `qccg`，Base URL 填入：

```
http://127.0.0.1:8963
```

## 从源码构建

```bash
go install github.com/wailsapp/wails/v3/cmd/wails@latest
cd frontend && npm ci && cd ..
wails build
```

## 鸣谢

- [qoder2api](https://github.com/cubk1/qoder2api) — 原始项目
- [qodercli-reverse](https://github.com/alingse/qodercli-reverse) — Qoder API 接口参考
- [Cola](https://colaos.ai) — miitmproxy 抓包、架构重构、签名算法还原
