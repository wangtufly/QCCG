#!/usr/bin/env bash
# 构建并打包 macOS dmg，自动处理 ad-hoc 签名
# 用法: ./build/package.sh

set -euo pipefail

# ---------- 配置 ----------
APP_NAME="qccg"
PRODUCT_NAME="QCCG"
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${PROJECT_ROOT}/build/bin"
APP_BUNDLE="${BIN_DIR}/${APP_NAME}.app"
TIMESTAMP="$(date +%Y%m%d_%H%M%S)"
DMG_NAME="${PRODUCT_NAME}_${TIMESTAMP}.dmg"
DMG_PATH="${BIN_DIR}/${DMG_NAME}"
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo 'dev')}"
CERT_NAME="QCCG Self-Signed"
KEYCHAIN_PATH="$HOME/Library/Keychains/qccg-codesign.keychain-db"

cd "${PROJECT_ROOT}"

# ---------- 1. 构建前端 ----------
echo "==> frontend build"
cd "${PROJECT_ROOT}/frontend"
npm run build
cd "${PROJECT_ROOT}"

# ---------- 2. go build ----------
echo "==> go build version=${VERSION}"
mkdir -p "${APP_BUNDLE}/Contents/MacOS"
go build -tags production -trimpath \
  -ldflags="-w -s -X qccg/internal/updater.Version=${VERSION}" \
  -o "${APP_BUNDLE}/Contents/MacOS/${APP_NAME}"

if [[ ! -f "${APP_BUNDLE}/Contents/MacOS/${APP_NAME}" ]]; then
  echo "构建失败: 未生成可执行文件"
  exit 1
fi

# ---------- 3. 使用 QCCG 自签名证书签名 ----------
# 若无证书，先运行 build/setup-codesign-cert.sh
# 确保 keychain 在搜索列表中
security list-keychains -s "$KEYCHAIN_PATH" "$HOME/Library/Keychains/login.keychain-db" /Library/Keychains/System.keychain 2>/dev/null || true
security unlock-keychain -p qccg "$KEYCHAIN_PATH" 2>/dev/null || true

if security find-identity -v -p codesigning "$KEYCHAIN_PATH" 2>/dev/null | grep -q "$CERT_NAME"; then
  echo "==> QCCG 自签名 .app"
  security unlock-keychain -p qccg "$KEYCHAIN_PATH" 2>/dev/null || true
  codesign --force --deep --sign "$CERT_NAME" --keychain "$KEYCHAIN_PATH" "${APP_BUNDLE}"
  codesign --verify --deep --strict --verbose=1 "${APP_BUNDLE}"
  echo "  签名身份保持固定，更新后权限不丢失"
else
  echo "==> ad-hoc 签名 .app（无 QCCG 证书，运行 build/setup-codesign-cert.sh 创建）"
  codesign --force --deep --sign - "${APP_BUNDLE}"
  codesign --verify --deep --strict --verbose=1 "${APP_BUNDLE}"
fi

# ---------- 4. 生成 dmg（create-dmg）----------
# 使用 create-dmg 替代 hdiutil，自动生成带 Applications 软链的拖拽安装窗口
echo "==> 创建 dmg (create-dmg)"
rm -f "${DMG_PATH}"
# create-dmg 在校验阶段失败时会返回非零，但 dmg 实际已生成，所以暂时关 set -e
set +e
create-dmg \
  --volname "${PRODUCT_NAME}" \
  --window-pos 200 120 \
  --window-size 600 400 \
  --icon-size 100 \
  --icon "${APP_NAME}.app" 175 190 \
  --hide-extension "${APP_NAME}.app" \
  --app-drop-link 425 190 \
  --no-internet-enable \
  "${DMG_PATH}" \
  "${APP_BUNDLE}"
CREATE_DMG_EXIT=$?
set -e
if [[ ! -f "${DMG_PATH}" ]]; then
  echo "create-dmg 失败 (exit ${CREATE_DMG_EXIT}): 未生成 ${DMG_PATH}"
  exit 1
fi

# ---------- 5. 签名 dmg ----------
if security find-identity -v -p codesigning "$KEYCHAIN_PATH" 2>/dev/null | grep -q "$CERT_NAME"; then
  echo "==> QCCG 自签名 dmg"
  security unlock-keychain -p qccg "$KEYCHAIN_PATH" 2>/dev/null || true
  codesign --force --sign "$CERT_NAME" --keychain "$KEYCHAIN_PATH" "${DMG_PATH}"
else
  echo "==> ad-hoc 签名 dmg"
  codesign --force --sign - "${DMG_PATH}"
fi

# ---------- 6. 移除 quarantine 属性（本机已有的话） ----------
xattr -dr com.apple.quarantine "${DMG_PATH}" 2>/dev/null || true

echo
echo "✅ 打包完成: ${DMG_PATH}"
echo
echo "⚠️  分发说明: 由于使用 ad-hoc 签名（未购买 Apple Developer 证书），"
echo "    其他用户首次打开时会被 Gatekeeper 拦截。让用户执行一次:"
echo "      sudo xattr -dr com.apple.quarantine /Applications/${PRODUCT_NAME}.app"
echo "    或下载后直接在 dmg 上执行:"
echo "      xattr -dr com.apple.quarantine ~/Downloads/${DMG_NAME}"