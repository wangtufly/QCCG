# QCCG 发布流程指南

## 📦 创建新版本

### 1. 准备发布

```bash
# 确保所有更改已提交
git add -A
git commit -m "Prepare for release vX.Y.Z"

# 推送至远程
git push origin main
```

### 2. 创建版本 Tag

```bash
# 创建带注释的 tag
git tag -a vX.Y.Z -m "Release vX.Y.Z - Brief description"

# 推送 tag（触发自动构建）
git push origin vX.Y.Z
```

### 3. 自动构建流程

推送 tag 后，GitHub Actions 会自动：

1. ✅ 在 macOS (Intel + Apple Silicon) 上构建
2. ✅ 在 Linux (x64) 上构建
3. ✅ 创建 DMG 安装包（macOS）
4. ✅ 创建 tar.gz 压缩包（Linux）
5. ✅ 创建 GitHub Release 草稿

### 4. 完成发布

1. 访问 GitHub 仓库 → **Releases** 页面
2. 找到刚创建的草稿发布
3. 编辑发布说明（可选）
4. 点击 **"Publish release"** 正式发布

## 🔧 手动触发构建

如果需要在没有 tag 的情况下测试构建流程：

1. 访问仓库 → **Actions** 标签
2. 选择 **"Build and Release"** 工作流
3. 点击 **"Run workflow"**
4. 输入版本号（如 `v1.0.0-test`）
5. 点击运行

## 📋 版本号规范

遵循 [Semantic Versioning](https://semver.org/)：

- **主版本 (X.y.z)**: 不兼容的 API 变更
- **次版本 (x.Y.z)**: 向后兼容的功能新增
- **修订版本 (x.y.Z)**: 向后兼容的问题修正

示例：
```bash
git tag -a v1.0.0 -m "Initial release"
git tag -a v1.1.0 -m "Add OAuth support"
git tag -a v1.0.1 -m "Fix bridge connection issue"
```

## 🎯 构建产物

每个版本会生成以下文件：

| 平台 | 架构 | 文件 |
|------|------|------|
| macOS | Intel (x64) | `QCCG-vX.Y.Z-darwin-amd64.dmg` |
| macOS | Apple Silicon | `QCCG-vX.Y.Z-darwin-arm64.dmg` |
| Linux | x64 | `qccg-vX.Y.Z-linux-amd64.tar.gz` |

## 💡 注意事项

### macOS 公证

如果要面向公众发布，建议：

1. 申请 Apple Developer 证书
2. 在 CI 中添加代码签名步骤
3. 提交 notarization（公证）

### 更新 wails.json 版本号

发布前记得更新 `wails.json` 中的版本号：

```json
{
  "info": {
    "productVersion": "1.0.0"
  }
}
```

### 清理本地构建产物

```bash
# 清理 Wails 构建目录
rm -rf build/bin/*

# 清理前端构建产物
rm -rf frontend/dist/*
```

## 🚀 首次发布清单

- [ ] 测试所有核心功能
- [ ] 更新 `wails.json` 版本号
- [ ] 更新 README 变更日志
- [ ] 创建版本 tag
- [ ] 推送 tag 触发构建
- [ ] 检查 GitHub Actions 构建结果
- [ ] 下载并测试构建产物
- [ ] 发布 GitHub Release

## 📞 问题排查

### 构建失败

1. 检查 Actions 日志
2. 确认依赖版本正确
3. 本地运行 `wails build` 测试

### DMG 创建失败

工作流会回退到只上传 `.app` 包，这是正常的。

### 发布产物缺失

检查 artifact 上传步骤是否成功，查看 Actions 日志。

---

**提示**: 第一个版本 `v1.0.0` 已经创建并推送，GitHub Actions 正在构建中！
