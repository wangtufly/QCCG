// Package updater 提供 QCCG 自动更新功能（方案 D：自签名 + 原地替换）
//
// 流程：
//  1. 启动时检查 GitHub Releases 最新版本
//  2. 若有新版本，前端弹出更新提示
//  3. 用户确认后下载 dmg，原地替换 .app，重签，重启
package updater

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Version 当前编译进来的版本号，由 -ldflags 注入。
var Version = "0.0.0-dev"

// Repo 检查更新的 GitHub 仓库 "owner/repo"。
var Repo = "wangtufly/QCCG"

// mirrors 多镜像列表，竞速下载时并发尝试。
// 格式为前缀式：https://{mirror}/{原始完整URL}
var mirrors = []string{
	"gh-proxy.com",
	"ghproxy.net",
	"github.akams.cn",
	"cdn.gh-proxy.org",
}

// buildMirrorURLs 生成所有候选下载地址（镜像 + 直连）。
func buildMirrorURLs(raw string) []string {
	env := os.Getenv("GITHUB_MIRROR")
	if env == "-" {
		return []string{raw}
	}
	var urls []string
	for _, m := range mirrors {
		urls = append(urls, "https://"+m+"/"+raw)
	}
	urls = append(urls, raw)
	return urls
}

// GitHubRelease 代表 GitHub Releases API 返回的 release 结构。
type GitHubRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

// UpdateInfo 返回给前端的更新信息。
type UpdateInfo struct {
	HasUpdate   bool   `json:"has_update"`
	ForceUpdate bool   `json:"force_update"`
	Current     string `json:"current"`
	Latest      string `json:"latest"`
	Body        string `json:"body"`
	DownloadURL string `json:"download_url"`
	FileSize    int64  `json:"file_size"`
}

// Check 检查是否有新版本。
func Check() (*UpdateInfo, error) {
	latest, err := fetchLatest()
	if err != nil {
		return nil, fmt.Errorf("检查更新失败: %w", err)
	}

	info := &UpdateInfo{
		Current: Version,
		Latest:  latest.TagName,
		Body:    latest.Body,
	}

	// 检测 release body 中的强制更新标记（dev 版本忽略）
	if strings.Contains(latest.Body, "<!-- force -->") && !strings.Contains(Version, "dev") {
		info.ForceUpdate = true
	}

	if latest.TagName == "" || latest.TagName == Version || latest.TagName == "v"+Version {
		info.HasUpdate = false
		return info, nil
	}

	// 按优先级匹配 dmg：当前架构 > universal > 任意 darwin
	arch := runtime.GOARCH // arm64 或 amd64
	var bestURL string
	var bestSize int64
	var priority int // 1=任意darwin 2=universal 3=当前架构
	for _, a := range latest.Assets {
		name := strings.ToLower(a.Name)
		if !strings.HasSuffix(name, ".dmg") {
			continue
		}
		if strings.Contains(name, arch) && priority < 3 {
			bestURL, bestSize, priority = a.BrowserDownloadURL, a.Size, 3
		} else if strings.Contains(name, "universal") && priority < 2 {
			bestURL, bestSize, priority = a.BrowserDownloadURL, a.Size, 2
		} else if strings.Contains(name, "darwin") && priority < 1 {
			bestURL, bestSize, priority = a.BrowserDownloadURL, a.Size, 1
		}
	}
	if bestURL != "" {
		info.DownloadURL = bestURL
		info.FileSize = bestSize
		info.HasUpdate = true
	}

	return info, nil
}

// Apply 下载更新并原地替换当前 app。
// 返回 true 表示已成功触发重启，调用方不应继续执行后续逻辑。
func Apply(downloadURL string, onProgress func(pct int)) (bool, error) {
	currentPath, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("获取当前可执行文件路径失败: %w", err)
	}

	// 从可执行文件路径推导 .app 路径
	// /Applications/QCCG.app/Contents/MacOS/qccg → /Applications/QCCG.app
	if runtime.GOOS != "darwin" {
		return false, fmt.Errorf("自动更新仅支持 macOS")
	}
	appPath := currentPath
	for i := 0; i < 3; i++ {
		appPath = filepath.Dir(appPath)
	}
	if !strings.HasSuffix(appPath, ".app") {
		return false, fmt.Errorf("无法确定 .app 路径: %s → %s", currentPath, appPath)
	}

	// 检查是否有预下载缓存
	cachedDMG := cachedDMGPath(downloadURL)
	hasCached := false
	if cachedDMG != "" {
		if fi, err := os.Stat(cachedDMG); err == nil && fi.Size() > 1024 && isDMGValid(cachedDMG) {
			hasCached = true
		}
	}

	tmpDir, err := os.MkdirTemp("", "qccg-update")
	if err != nil {
		return false, fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	dmgPath := filepath.Join(tmpDir, "update.dmg")

	if hasCached {
		// 直接用缓存，跳过下载
		if onProgress != nil {
			onProgress(5)
		}
		if err := os.Link(cachedDMG, dmgPath); err != nil {
			// 硬链接失败则拷贝
			data, readErr := os.ReadFile(cachedDMG)
			if readErr != nil {
				return false, fmt.Errorf("读取缓存失败: %w", readErr)
			}
			if writeErr := os.WriteFile(dmgPath, data, 0644); writeErr != nil {
				return false, fmt.Errorf("写入 dmg 失败: %w", writeErr)
			}
		}
		if onProgress != nil {
			onProgress(55)
		}
	} else {
		// 无缓存，正常下载
		if onProgress != nil {
			onProgress(5)
		}
		if err := downloadFile(downloadURL, dmgPath, onProgress); err != nil {
			return false, fmt.Errorf("下载失败: %w", err)
		}
	}

	if onProgress != nil {
		onProgress(60)
	}

	// 挂载 dmg
	mountPoint := filepath.Join(tmpDir, "mnt")
	if err := mountDMG(dmgPath, mountPoint); err != nil {
		return false, fmt.Errorf("挂载 dmg 失败: %w", err)
	}
	defer func() { _ = unmountDMG(mountPoint) }()

	// 查找 dmg 中的 .app
	newApp, err := findAppInDir(mountPoint)
	if err != nil {
		return false, fmt.Errorf("dmg 中未找到 .app: %w", err)
	}

	if onProgress != nil {
		onProgress(75)
	}

	// 原地替换：先删旧 .app，再拷贝新 .app
	tmpApp := filepath.Join(tmpDir, "new.app")
	if err := copyDir(newApp, tmpApp); err != nil {
		return false, fmt.Errorf("复制新版本失败: %w", err)
	}

	if onProgress != nil {
		onProgress(85)
	}

	// 重签新 app
	if err := resignApp(tmpApp); err != nil {
		return false, fmt.Errorf("重签失败: %w", err)
	}

	if onProgress != nil {
		onProgress(95)
	}

	// 把 newApp 和脚本移到 tmpDir 外，避免 defer RemoveAll 删掉还没跑完的文件
	persistDir, err2 := os.MkdirTemp("", "qccg-replace")
	if err2 != nil {
		return false, fmt.Errorf("创建替换目录失败: %w", err2)
	}
	persistApp := filepath.Join(persistDir, "new.app")
	if renameErr := os.Rename(tmpApp, persistApp); renameErr != nil {
		if copyErr := copyDir(tmpApp, persistApp); copyErr != nil {
			return false, fmt.Errorf("迁移新版本失败: %w", copyErr)
		}
	}

	// 生成替换脚本（在 app 退出后执行）
	scriptPath := filepath.Join(persistDir, "replace.sh")
	script := replaceScript(persistApp, appPath, persistDir)
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return false, fmt.Errorf("写入替换脚本失败: %w", err)
	}

	// 非阻塞执行替换脚本 → 替换旧 app → 启动新 app
	cmd := exec.Command("/bin/sh", scriptPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf("启动替换脚本失败: %w", err)
	}

	if onProgress != nil {
		onProgress(100)
	}

	return true, nil
}

// -------- internals --------

func fetchLatest() (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", Repo)

	client := &http.Client{Timeout: 60 * time.Second}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "QCCG-Updater/1.0")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if attempt < 3 {
				time.Sleep(time.Duration(attempt) * 2 * time.Second)
				continue
			}
			return nil, fmt.Errorf("检查更新失败（重试%d次后）: %w", attempt, lastErr)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			resp.Body.Close()
			lastErr = fmt.Errorf("GitHub API 返回 %d", resp.StatusCode)
			if attempt < 3 {
				time.Sleep(time.Duration(attempt) * 2 * time.Second)
				continue
			}
			return nil, fmt.Errorf("检查更新失败（重试%d次后）: %w", attempt, lastErr)
		}

		var release GitHubRelease
		if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
			return nil, err
		}
		return &release, nil
	}
	return nil, fmt.Errorf("检查更新失败（重试后仍失败）: %w", lastErr)
}

func downloadFile(url, dest string, onProgress func(pct int)) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	urls := buildMirrorURLs(url)

	// 竞速：并发请求所有镜像，第一个成功返回 200 的胜出
	type result struct {
		resp   *http.Response
		cancel context.CancelFunc
		err    error
	}

	ch := make(chan result, len(urls))
	for _, dlURL := range urls {
		go func(u string) {
			reqCtx, reqCancel := context.WithCancel(ctx)
			req, err := http.NewRequestWithContext(reqCtx, "GET", u, nil)
			if err != nil {
				reqCancel()
				ch <- result{nil, nil, err}
				return
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				reqCancel()
				ch <- result{nil, nil, err}
				return
			}
			if resp.StatusCode != 200 {
				resp.Body.Close()
				reqCancel()
				ch <- result{nil, nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, u)}
				return
			}
			ch <- result{resp, reqCancel, nil}
		}(dlURL)
	}

	// 收集结果，取第一个成功的
	var winner *http.Response
	var winnerCancel context.CancelFunc
	var errs []error
	for range urls {
		r := <-ch
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		if winner == nil {
			winner = r.resp
			winnerCancel = r.cancel
		} else {
			// 取消落败请求
			r.cancel()
			r.resp.Body.Close()
		}
	}

	if winner == nil {
		return fmt.Errorf("所有镜像下载失败: %v", errs)
	}
	defer winner.Body.Close()
	defer winnerCancel()

	return writeResponseBody(winner, dest, onProgress, ctx)
}

// writeResponseBody 将 HTTP 响应体写入文件，带进度回调。
func writeResponseBody(resp *http.Response, dest string, onProgress func(pct int), ctx context.Context) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	total := resp.ContentLength // -1 表示未知
	var downloaded int64
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			downloaded += int64(n)
			if onProgress != nil && total > 0 {
				// 下载占总进度 5%−55%
				pct := 5 + int(float64(downloaded)/float64(total)*50)
				if pct > 55 {
					pct = 55
				}
				onProgress(pct)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			// 检查是否超时
			if ctx.Err() != nil {
				return fmt.Errorf("下载超时（已下载 %.1f MB / %.1f MB）: %w",
					float64(downloaded)/1024/1024,
					float64(total)/1024/1024,
					ctx.Err())
			}
			return readErr
		}
	}
	return nil
}

func mountDMG(dmgPath, mountPoint string) error {
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		return err
	}
	cmd := exec.Command("hdiutil", "attach", dmgPath, "-mountpoint", mountPoint, "-nobrowse", "-quiet")
	return cmd.Run()
}

func unmountDMG(mountPoint string) error {
	return exec.Command("hdiutil", "detach", mountPoint, "-force", "-quiet").Run()
}

func findAppInDir(dir string) (string, error) {
	var found string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasSuffix(path, ".app") && info.IsDir() {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("未找到 .app")
	}
	return found, nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		dest := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(dest, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, data, info.Mode())
	})
}

func resignApp(appPath string) error {
	// 更新时用 ad-hoc 签名，避免运行期 keychain 密码弹窗。
	// 构建时已用固定的自签名证书签名，运行时重签 ad-hoc 不影响已存数据和权限。
	cmd := exec.Command("codesign", "--force", "--deep", "--sign", "-", appPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("codesign: %s: %w", string(output), err)
	}
	return nil
}

// replaceScript 生成 bash 脚本：等待 app 退出 → 替换 → 启动新 app。
func replaceScript(newApp, oldApp, persistDir string) string {
	pid := os.Getpid()
	return fmt.Sprintf(`#!/bin/bash
set -e
# 等待旧进程退出（按 PID 判断，避免路径空格问题）
sleep 0.5
while kill -0 %d 2>/dev/null; do sleep 0.3; done
# 移除旧 app
rm -rf "%s"
# 移动新 app
mv "%s" "%s"
# 移除 quarantine
xattr -dr com.apple.quarantine "%s" 2>/dev/null || true
# 启动新 app
open "%s"
# 清理临时目录并删脚本
rm -f "$0" && rmdir "%s" 2>/dev/null || true
`, pid, oldApp, newApp, oldApp, oldApp, oldApp, persistDir)
}

// -------- 预下载 --------

// isDMGValid 检查文件尾部是否包含 DMG trailer（koly 魔数）。
func isDMGValid(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	// DMG 文件尾部 512 字节内包含 "koly" 魔数
	fi, _ := f.Stat()
	if fi.Size() < 512 {
		return false
	}
	buf := make([]byte, 512)
	if _, err := f.ReadAt(buf, fi.Size()-512); err != nil {
		return false
	}
	for i := 0; i < len(buf)-4; i++ {
		if buf[i] == 'k' && buf[i+1] == 'o' && buf[i+2] == 'l' && buf[i+3] == 'y' {
			return true
		}
	}
	return false
}

// cacheDir 返回预下载缓存目录。
func cacheDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, "Library", "Caches", "QCCG", "updates")
	os.MkdirAll(dir, 0755)
	return dir
}

// cachedDMGPath 根据下载 URL 推导缓存文件路径。
func cachedDMGPath(downloadURL string) string {
	if downloadURL == "" {
		return ""
	}
	// 用文件名作为缓存 key
	parts := strings.Split(downloadURL, "/")
	name := parts[len(parts)-1]
	if name == "" {
		return ""
	}
	return filepath.Join(cacheDir(), name)
}

// PreDownload 静默预下载更新到本地缓存，返回下载结果。
// 调用方应在后台 goroutine 中调用。
func PreDownload(downloadURL string) error {
	if downloadURL == "" {
		return fmt.Errorf("空下载地址")
	}
	dest := cachedDMGPath(downloadURL)
	if dest == "" {
		return fmt.Errorf("无法确定缓存路径")
	}
	// 已缓存则跳过
	if fi, err := os.Stat(dest); err == nil && fi.Size() > 0 {
		return nil
	}
	return downloadFile(downloadURL, dest, nil)
}

// CleanCache 清理过期缓存（保留最新一个）。
func CleanCache() {
	dir := cacheDir()
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) <= 1 {
		return
	}
	// 按修改时间排序，保留最新
	type fileEntry struct {
		path    string
		modTime time.Time
	}
	var files []fileEntry
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileEntry{filepath.Join(dir, e.Name()), info.ModTime()})
	}
	if len(files) <= 1 {
		return
	}
	// 找最新的
	newest := files[0]
	for _, f := range files[1:] {
		if f.modTime.After(newest.modTime) {
			newest = f
		}
	}
	// 删除其余
	for _, f := range files {
		if f.path != newest.path {
			os.Remove(f.path)
		}
	}
}

// zipExtract 保留备用，当前 dmg 方案不使用 zip。
func zipExtract(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		path := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal path: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(path, f.Mode())
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		w, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(w, rc)
		rc.Close()
		w.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
