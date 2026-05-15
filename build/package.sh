#!/usr/bin/env bash
# 构建并打包 macOS dmg，自动处理 ad-hoc 签名
# 用法: ./build/package.sh

set -euo pipefail

# ---------- 配置 ----------
APP_NAME="qoder2api"
PRODUCT_NAME="Qoder2API"
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${PROJECT_ROOT}/build/bin"
APP_BUNDLE="${BIN_DIR}/${APP_NAME}.app"
TIMESTAMP="$(date +%Y%m%d_%H%M%S)"
DMG_NAME="${PRODUCT_NAME}_${TIMESTAMP}.dmg"
DMG_PATH="${BIN_DIR}/${DMG_NAME}"

cd "${PROJECT_ROOT}"

# ---------- 1. wails 构建 ----------
echo "==> wails build (universal)"
# universal: 同时打包 arm64 + amd64，兼容所有 Mac
wails build -platform darwin/universal -clean

if [[ ! -d "${APP_BUNDLE}" ]]; then
  echo "构建失败: 未生成 ${APP_BUNDLE}"
  exit 1
fi

# ---------- 2. 重新 ad-hoc 签名整个 .app ----------
# wails 默认签名只覆盖主可执行文件，--deep 确保所有内嵌资源都被签到
echo "==> ad-hoc 签名 .app"
codesign --force --deep --sign - "${APP_BUNDLE}"
codesign --verify --deep --strict --verbose=2 "${APP_BUNDLE}"

# ---------- 3. 生成 dmg ----------
echo "==> 创建 dmg"
rm -f "${DMG_PATH}"
hdiutil create \
  -volname "${PRODUCT_NAME}" \
  -srcfolder "${APP_BUNDLE}" \
  -ov \
  -format UDZO \
  -imagekey zlib-level=9 \
  "${DMG_PATH}"

# ---------- 4. 给 dmg 自身也加 ad-hoc 签名 ----------
echo "==> ad-hoc 签名 dmg"
codesign --force --sign - "${DMG_PATH}"

# ---------- 5. 移除 quarantine 属性（本机已有的话） ----------
xattr -dr com.apple.quarantine "${DMG_PATH}" 2>/dev/null || true

echo
echo "✅ 打包完成: ${DMG_PATH}"
echo
echo "⚠️  分发说明: 由于使用 ad-hoc 签名（未购买 Apple Developer 证书），"
echo "    其他用户首次打开时会被 Gatekeeper 拦截。让用户执行一次:"
echo "      sudo xattr -dr com.apple.quarantine /Applications/${PRODUCT_NAME}.app"
echo "    或下载后直接在 dmg 上执行:"
echo "      xattr -dr com.apple.quarantine ~/Downloads/${DMG_NAME}"