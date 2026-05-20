#!/usr/bin/env bash
# 生成并导入 QCCG 自签名代码证书到独立 keychain
# 运行一次即可，证书有效期 10 年。macOS 15+ 兼容。
set -euo pipefail

CERT_NAME="QCCG Self-Signed"
KEYCHAIN_PATH="$HOME/Library/Keychains/qccg-codesign.keychain-db"
KEYCHAIN_PASS="qccg"

echo "==> 生成证书和密钥..."
cat > /tmp/qccg-codesign.conf << 'CEOF'
[ req ]
default_bits = 2048
distinguished_name = dn
prompt = no
x509_extensions = codesign_ext
[ dn ]
CN = QCCG Self-Signed
O = QCCG
OU = Development
[ codesign_ext ]
basicConstraints = CA:FALSE
keyUsage = critical,digitalSignature
extendedKeyUsage = codeSigning
subjectKeyIdentifier = hash
CEOF

openssl req -x509 -newkey rsa:2048 \
  -keyout /tmp/qccg-codesign-key.pem \
  -out /tmp/qccg-codesign-cert.pem \
  -days 3650 -nodes \
  -config /tmp/qccg-codesign.conf \
  -extensions codesign_ext 2>/dev/null

echo "==> 创建专用 keychain..."
security delete-keychain "$KEYCHAIN_PATH" 2>/dev/null || true
security create-keychain -p "$KEYCHAIN_PASS" "$KEYCHAIN_PATH"
security unlock-keychain -p "$KEYCHAIN_PASS" "$KEYCHAIN_PATH"

echo "==> 导入私钥和证书..."
security import /tmp/qccg-codesign-key.pem -k "$KEYCHAIN_PATH" -x -A -T /usr/bin/codesign
security import /tmp/qccg-codesign-cert.pem -k "$KEYCHAIN_PATH" -A -T /usr/bin/codesign

echo "==> 信任证书为代码签名根证书..."
security add-trusted-cert -d -r trustRoot -p codeSign -k "$KEYCHAIN_PATH" /tmp/qccg-codesign-cert.pem

echo "==> 设置访问权限（macOS 15+ 必需）..."
security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "$KEYCHAIN_PASS" "$KEYCHAIN_PATH"

echo "==> 验证..."
if security find-identity -v -p codesigning "$KEYCHAIN_PATH" 2>&1 | grep -q "$CERT_NAME"; then
    echo "✅ 证书就绪: $CERT_NAME"
    echo "   位置: $KEYCHAIN_PATH"
    echo "   签名命令: codesign --sign '$CERT_NAME' --keychain '$KEYCHAIN_PATH' <文件>"
else
    echo "❌ 证书创建失败"
    exit 1
fi

# 清理临时文件
rm -f /tmp/qccg-codesign.conf /tmp/qccg-codesign-key.pem /tmp/qccg-codesign-cert.pem
