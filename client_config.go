package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"qccg/logger"
)

// ClientConfig 是返回前端的客户端配置状态。
//   - ConfigPath/BaseURL/EnvVars 仅作展示
//   - Applied 表示「已经被 qccg 标注过」(由 Marker 字段判断，避免误判用户原本就有的配置)
//   - Model 是当前配置文件里读出来的「主力」模型名（仅展示）
type ClientConfig struct {
	Type       string `json:"type"`
	Name       string `json:"name"`
	Icon       string `json:"icon"`
	ConfigPath string `json:"config_path"`
	BaseURL    string `json:"base_url"`
	EnvVars    string `json:"env_vars"`
	Model      string `json:"model"`
	Applied    bool   `json:"applied"`
	Error      string `json:"error,omitempty"`
}

// 内部 marker：用于识别「这一段配置是 qccg 写入的」，
// 移除时只移除自己写的部分，不动用户其它配置。
const (
	qoderProviderID    = "qccg"
	defaultQoderAPIKey = "qccg"
	codexProviderURL   = "/v1"
)

func (a *App) effectiveToken() string {
	if a.bridgeToken != "" {
		return a.bridgeToken
	}
	return defaultQoderAPIKey
}

func hasBackupFile(home, clientType string) bool {
	if _, err := os.Stat(backupPath(home, clientType)); err == nil {
		return true
	}
	if clientType == "codex" {
		if _, err := os.Stat(codexAuthBackupPath(home)); err == nil {
			return true
		}
		if _, err := os.Stat(codexAuthMissingMarkerPath(home)); err == nil {
			return true
		}
	}
	return false
}

func bridgeBaseURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

func (a *App) GetClientConfigs() []ClientConfig {
	port := a.bridgePort
	token := a.effectiveToken()
	home, _ := os.UserHomeDir()

	configs := []ClientConfig{
		{
			Type:       "claude",
			Name:       "Claude Code",
			Icon:       "🧠",
			ConfigPath: filepath.Join(home, ".claude", "settings.json"),
			BaseURL:    bridgeBaseURL(port),
			EnvVars:    fmt.Sprintf(`export ANTHROPIC_BASE_URL="http://127.0.0.1:%d"`+"\n"+`export ANTHROPIC_AUTH_TOKEN="%s"`, port, token),
		},
		{
			Type:       "codex",
			Name:       "Codex CLI",
			Icon:       "⚡",
			ConfigPath: filepath.Join(home, ".codex", "config.toml"),
			BaseURL:    bridgeBaseURL(port) + codexProviderURL,
			EnvVars:    "# Codex 不依赖环境变量，配置写入 ~/.codex/config.toml + auth.json 即可",
		},
		{
			Type:       "gemini",
			Name:       "Gemini CLI",
			Icon:       "🌟",
			ConfigPath: filepath.Join(home, ".gemini", ".env"),
			BaseURL:    bridgeBaseURL(port),
			EnvVars:    fmt.Sprintf(`export GOOGLE_GEMINI_BASE_URL="http://127.0.0.1:%d"`+"\n"+`export GEMINI_API_KEY="%s"`, port, token),
		},
	}

	for i := range configs {
		cfg := &configs[i]
		applied, model, err := readClientStatus(cfg.Type, home)
		cfg.Applied = applied
		cfg.Model = model
		if err != nil {
			cfg.Error = err.Error()
		}
	}
	return configs
}

func (a *App) ApplyClientConfig(clientType, model string) error {
	port := a.bridgePort
	token := a.effectiveToken()
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	switch clientType {
	case "claude":
		return writeClaudeConfig(home, port, token, model)
	case "codex":
		return writeCodexConfig(home, port, token, model)
	case "gemini":
		return writeGeminiConfig(home, port, token, model)
	}
	return fmt.Errorf("unknown client type: %s", clientType)
}

// ClientConfigFile 是返回前端的「主配置文件原文 + 路径」结构，用于编辑器展示
type ClientConfigFile struct {
	Path       string             `json:"path"`
	Content    string             `json:"content"`
	Format     string             `json:"format"` // "json" / "toml" / "dotenv"
	Existed    bool               `json:"existed"`
	ExtraFiles []ClientConfigFile `json:"extra_files,omitempty"`
}

func backupPath(home, clientType string) string {
	return filepath.Join(home, ".qccg", "backups", clientType+"_config.bak")
}

func codexAuthBackupPath(home string) string {
	return filepath.Join(home, ".qccg", "backups", "codex_auth.bak")
}

func codexAuthMissingMarkerPath(home string) string {
	return filepath.Join(home, ".qccg", "backups", "codex_auth.missing")
}

// BackupClientConfigFile 在保存前把原文件备份到 ~/.qccg/backups/<type>_config.bak
func (a *App) BackupClientConfigFile(clientType string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	src, _ := mainConfigPath(home, clientType)
	if src == "" {
		return fmt.Errorf("unknown client type: %s", clientType)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	} else {
		dst := backupPath(home, clientType)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := atomicWriteFile(dst, data, 0o644); err != nil {
			return err
		}
	}

	if clientType == "codex" {
		authSrc := filepath.Join(home, ".codex", "auth.json")
		authBak := codexAuthBackupPath(home)
		missingMarker := codexAuthMissingMarkerPath(home)
		authData, authErr := os.ReadFile(authSrc)
		if authErr != nil {
			if !os.IsNotExist(authErr) {
				return authErr
			}
			if err := os.MkdirAll(filepath.Dir(missingMarker), 0o755); err != nil {
				return err
			}
			if err := atomicWriteFile(missingMarker, []byte("missing"), 0o644); err != nil {
				return err
			}
			_ = os.Remove(authBak)
		} else {
			if err := os.MkdirAll(filepath.Dir(authBak), 0o755); err != nil {
				return err
			}
			if err := atomicWriteFile(authBak, authData, 0o644); err != nil {
				return err
			}
			_ = os.Remove(missingMarker)
		}
	}
	return nil
}

// HasClientConfigBackup 返回指定 client 是否存在备份文件
func (a *App) HasClientConfigBackup(clientType string) bool {
	home, _ := os.UserHomeDir()
	return hasBackupFile(home, clientType)
}

// RestoreClientConfigFile 把备份文件还原回原路径，还原后删除备份
func (a *App) RestoreClientConfigFile(clientType string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dst, _ := mainConfigPath(home, clientType)
	if dst == "" {
		return fmt.Errorf("unknown client type: %s", clientType)
	}
	bak := backupPath(home, clientType)
	data, err := os.ReadFile(bak)
	if err != nil {
		return fmt.Errorf("no backup found: %w", err)
	}
	if err := atomicWriteFile(dst, data, 0o644); err != nil {
		return err
	}
	_ = os.Remove(bak)

	if clientType == "codex" {
		authPath := filepath.Join(home, ".codex", "auth.json")
		authBak := codexAuthBackupPath(home)
		missingMarker := codexAuthMissingMarkerPath(home)
		if authData, authErr := os.ReadFile(authBak); authErr == nil {
			if err := atomicWriteFile(authPath, authData, 0o644); err != nil {
				return err
			}
			_ = os.Remove(authBak)
			_ = os.Remove(missingMarker)
		} else if _, markerErr := os.Stat(missingMarker); markerErr == nil {
			_ = os.Remove(authPath)
			_ = os.Remove(missingMarker)
		}
	}

	logger.Info("Restored %s config from backup", clientType)
	return nil
}

// ReadClientConfigFile 读取指定 client 主配置文件原文（不做任何解析/合并），
// 用于前端编辑器展示。若文件不存在返回空 content + Existed=false（不是错误）。
func readSingleClientConfigFile(path, format string) (ClientConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ClientConfigFile{Path: path, Format: format, Existed: false}, nil
		}
		return ClientConfigFile{}, err
	}
	return ClientConfigFile{Path: path, Content: string(data), Format: format, Existed: true}, nil
}

func (a *App) ReadClientConfigFile(clientType string) (*ClientConfigFile, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path, format := mainConfigPath(home, clientType)
	if path == "" {
		return nil, fmt.Errorf("unknown client type: %s", clientType)
	}
	mainFile, err := readSingleClientConfigFile(path, format)
	if err != nil {
		return nil, err
	}
	result := mainFile
	if clientType == "codex" {
		extraPath := filepath.Join(home, ".codex", "auth.json")
		extra, err := readSingleClientConfigFile(extraPath, "json")
		if err != nil {
			return nil, err
		}
		result.ExtraFiles = []ClientConfigFile{extra}
	}
	return &result, nil
}

// validateConfigContent 对编辑器内容按 format 做语法校验。
// .env 不校验（dotenv 没有官方语法）；JSON / TOML 校验失败直接返回 error，
// 不写入磁盘，避免破坏原文件。
func validateConfigContent(format, content string) error {
	switch format {
	case "json":
		var v interface{}
		if err := json.Unmarshal([]byte(content), &v); err != nil {
			return fmt.Errorf("JSON 语法错误: %w", err)
		}
	case "toml":
		var v map[string]interface{}
		if err := toml.Unmarshal([]byte(content), &v); err != nil {
			return fmt.Errorf("TOML 语法错误: %w", err)
		}
	}
	return nil
}

func (a *App) SaveClientConfigFile(clientType, content string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path, format := mainConfigPath(home, clientType)
	if path == "" {
		return fmt.Errorf("unknown client type: %s", clientType)
	}
	if !a.HasClientConfigBackup(clientType) {
		if err := a.BackupClientConfigFile(clientType); err != nil {
			logger.Info("Backup skipped for %s: %v", clientType, err)
		}
	}
	if err := validateConfigContent(format, content); err != nil {
		return err
	}
	if err := atomicWriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("写入失败: %w", err)
	}
	logger.Info("Saved %s config file: %s (%d bytes)", clientType, path, len(content))
	return nil
}

func (a *App) SaveAdditionalClientConfigFile(clientType, path, format, content string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if clientType != "codex" {
		return fmt.Errorf("client type %s has no additional config files", clientType)
	}
	expectedPath := filepath.Join(home, ".codex", "auth.json")
	if path != expectedPath {
		return fmt.Errorf("unsupported additional config path: %s", path)
	}
	if !a.HasClientConfigBackup(clientType) {
		if err := a.BackupClientConfigFile(clientType); err != nil {
			logger.Info("Backup skipped for %s: %v", clientType, err)
		}
	}
	if err := validateConfigContent(format, content); err != nil {
		return err
	}
	if err := atomicWriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("写入失败: %w", err)
	}
	logger.Info("Saved additional %s config file: %s (%d bytes)", clientType, path, len(content))
	return nil
}

func mainConfigPath(home, clientType string) (path, format string) {
	switch clientType {
	case "claude":
		return filepath.Join(home, ".claude", "settings.json"), "json"
	case "codex":
		return filepath.Join(home, ".codex", "config.toml"), "toml"
	case "gemini":
		return filepath.Join(home, ".gemini", ".env"), "dotenv"
	}
	return "", ""
}

func (a *App) RemoveClientConfig(clientType string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	switch clientType {
	case "claude":
		return removeClaudeConfig(home)
	case "codex":
		return removeCodexConfig(home)
	case "gemini":
		return removeGeminiConfig(home)
	}
	return fmt.Errorf("unknown client type: %s", clientType)
}

// ===== 通用 helpers =====

func readJSONObjectOrEmpty(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]interface{}{}, nil
		}
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if m == nil {
		m = map[string]interface{}{}
	}
	return m, nil
}

func writeJSONObjectOrdered(path string, m map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := marshalJSONOrderPreserved(m)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// marshalJSONOrderPreserved 按预定义顺序输出 JSON 字段（避免 Go map 遍历顺序不确定的问题）
// Claude settings.json 的顶级键顺序：env, enabledPlugins, permissions, model, ...
var topLevelOrder = []string{"env", "enabledPlugins", "permissions", "model", "extensions", "hooks"}

// orderedKeys 计算 map 的稳定 key 顺序：predefined 顺序优先，其余按字母序。
func orderedKeys(m map[string]interface{}, predefined []string) []string {
	keys := make([]string, 0, len(m))
	used := make(map[string]bool, len(m))
	for _, k := range predefined {
		if _, ok := m[k]; ok {
			keys = append(keys, k)
			used[k] = true
		}
	}
	extras := make([]string, 0, len(m))
	for k := range m {
		if !used[k] {
			extras = append(extras, k)
		}
	}
	sort.Strings(extras)
	return append(keys, extras...)
}

// marshalJSONValue 递归地把任意 JSON 值序列化为带缩进的字节流。
// 与 json.MarshalIndent 的差异：1) 关闭 HTMLEscape，避免 hook 命令里的 >/& 被改写成
// >/&；2) map 的 key 顺序稳定（顶层按 topLevelOrder，其余按字母序），
// 避免每次写入因 Go map 遍历随机产生 diff 抖动。
//
// indent 是基础缩进单元（两个空格），prefix 是当前层级前缀。
func marshalJSONValue(v interface{}, indent, prefix string) ([]byte, error) {
	switch val := v.(type) {
	case map[string]interface{}:
		if len(val) == 0 {
			return []byte("{}"), nil
		}
		var b strings.Builder
		b.WriteString("{\n")
		childPrefix := prefix + indent
		keys := orderedKeys(val, nil)
		for i, k := range keys {
			child, err := marshalJSONValue(val[k], indent, childPrefix)
			if err != nil {
				return nil, err
			}
			keyBytes, err := encodeStringNoHTMLEscape(k)
			if err != nil {
				return nil, err
			}
			b.WriteString(childPrefix)
			b.Write(keyBytes)
			b.WriteString(": ")
			b.Write(child)
			if i < len(keys)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString(prefix)
		b.WriteByte('}')
		return []byte(b.String()), nil
	case []interface{}:
		if len(val) == 0 {
			return []byte("[]"), nil
		}
		var b strings.Builder
		b.WriteString("[\n")
		childPrefix := prefix + indent
		for i, item := range val {
			child, err := marshalJSONValue(item, indent, childPrefix)
			if err != nil {
				return nil, err
			}
			b.WriteString(childPrefix)
			b.Write(child)
			if i < len(val)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString(prefix)
		b.WriteByte(']')
		return []byte(b.String()), nil
	default:
		return encodeScalarNoHTMLEscape(v)
	}
}

// encodeStringNoHTMLEscape 把字符串用 JSON 规则编码，但不做 HTML 转义。
func encodeStringNoHTMLEscape(s string) ([]byte, error) {
	return encodeScalarNoHTMLEscape(s)
}

// encodeScalarNoHTMLEscape 序列化任意非 map/slice 标量值，关闭 HTMLEscape。
func encodeScalarNoHTMLEscape(v interface{}) ([]byte, error) {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	// json.Encoder.Encode 会追加一个换行，去掉。
	out := buf.String()
	out = strings.TrimRight(out, "\n")
	return []byte(out), nil
}

func marshalJSONOrderPreserved(v interface{}) ([]byte, error) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return marshalJSONValue(v, "  ", "")
	}
	if len(m) == 0 {
		return []byte("{}"), nil
	}
	var b strings.Builder
	b.WriteString("{\n")
	keys := orderedKeys(m, topLevelOrder)
	for i, k := range keys {
		child, err := marshalJSONValue(m[k], "  ", "  ")
		if err != nil {
			return nil, err
		}
		keyBytes, err := encodeStringNoHTMLEscape(k)
		if err != nil {
			return nil, err
		}
		b.WriteString("  ")
		b.Write(keyBytes)
		b.WriteString(": ")
		b.Write(child)
		if i < len(keys)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("}")
	return []byte(b.String()), nil
}

func readClientStatus(clientType, home string) (applied bool, model string, err error) {
	switch clientType {
	case "claude":
		return readClaudeStatus(home)
	case "codex":
		return readCodexStatus(home)
	case "gemini":
		return readGeminiStatus(home)
	}
	return false, "", nil
}

// ===== Claude Code =====
//
// Claude Code 读 ~/.claude/settings.json 中的 env 字段；schema:
//
//	{ "env": { "ANTHROPIC_BASE_URL": "...", "ANTHROPIC_AUTH_TOKEN": "...", ... } }
//
// 用于识别状态；其它字段（hooks/permissions/MCP 等）原样保留。

func writeClaudeConfig(home string, port int, token, model string) error {
	path := filepath.Join(home, ".claude", "settings.json")

	// 读取原始文件（预留，当前未使用但保持可读性）
	_ /*data*/, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	root, err := readJSONObjectOrEmpty(path)
	if err != nil {
		return err
	}

	env, _ := root["env"].(map[string]interface{})
	if env == nil {
		env = map[string]interface{}{}
	}
	env["ANTHROPIC_BASE_URL"] = bridgeBaseURL(port)
	env["ANTHROPIC_AUTH_TOKEN"] = token
	if model != "" {
		env["ANTHROPIC_MODEL"] = model
	}
	root["env"] = env

	// 按固定顺序输出 JSON 以避免字段顺序漂移
	if err := writeJSONObjectOrdered(path, root); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	logger.Info("Wrote Claude Code config: %s (model=%s)", path, model)
	return nil
}

func removeClaudeConfig(home string) error {
	path := filepath.Join(home, ".claude", "settings.json")
	root, err := readJSONObjectOrEmpty(path)
	if err != nil {
		return err
	}
	if env, ok := root["env"].(map[string]interface{}); ok {
		delete(env, "ANTHROPIC_BASE_URL")
		delete(env, "ANTHROPIC_AUTH_TOKEN")
		delete(env, "ANTHROPIC_MODEL")
		if len(env) == 0 {
			delete(root, "env")
		} else {
			root["env"] = env
		}
	}
	if len(root) == 0 {
		// 完全是我们写的，直接删文件
		_ = os.Remove(path)
		logger.Info("Removed Claude Code config: %s", path)
		return nil
	}
	if err := writeJSONObjectOrdered(path, root); err != nil {
		return err
	}
	logger.Info("Cleaned qccg fields from %s", path)
	return nil
}

func readClaudeStatus(home string) (bool, string, error) {
	if !hasBackupFile(home, "claude") {
		return false, "", nil
	}
	path := filepath.Join(home, ".claude", "settings.json")
	root, err := readJSONObjectOrEmpty(path)
	if err != nil {
		return true, "", nil
	}
	model := ""
	if env, ok := root["env"].(map[string]interface{}); ok {
		if v, ok := env["ANTHROPIC_MODEL"].(string); ok {
			model = v
		}
	}
	return true, model, nil
}

// ===== Codex CLI =====
//
// Codex 读 ~/.codex/config.toml + ~/.codex/auth.json。
// config.toml 关键字段：
//
//	model_provider = "qccg"
//	model = "gpt-5"  # 可选
//	[model_providers.qccg]
//	name = "QCCG"
//	base_url = "http://127.0.0.1:8963/v1"
//	wire_api = "responses"
//
// auth.json:
//
//	{ "OPENAI_API_KEY": "qccg" }
//
// 写配置时用字符串级 TOML 编辑（而非 Unmarshal → Marshal），
// 保留用户的注释、空行、MCP 配置、profiles 和其它 provider 节。
// model_provider ID 会做稳定化处理：如果用户的活跃 ID 不是保留字（openai/ollama 等），
// 就保留旧 ID 不变，避免 Codex 会话历史丢失。

// reservedCodexProviderIDs 是 Codex CLI 内置的 provider ID 列表。
// 当用户当前 model_provider 是其中一个时，QCCG 使用自己的 "qccg" ID；
// 否则保留用户的自定义 ID 以保持会话历史不丢失。
var reservedCodexProviderIDs = map[string]bool{
	"openai":         true,
	"ollama":         true,
	"lmstudio":       true,
	"oss":            true,
	"ollama-chat":    true,
	"amazon-bedrock": true,
	"qccg":           true,
}

func isReservedCodexProviderID(id string) bool {
	return reservedCodexProviderIDs[strings.ToLower(strings.TrimSpace(id))]
}

// determineCodexProviderID 解析现有 config.toml 文本，找出活跃的 model_provider。
// 如果是自定义（非保留）ID → 保留它（稳定化）；否则回落到 "qccg"。
func determineCodexProviderID(tomlText string) string {
	if tomlText == "" {
		return qoderProviderID
	}
	var doc map[string]interface{}
	if err := toml.Unmarshal([]byte(tomlText), &doc); err != nil {
		return qoderProviderID
	}
	mp, _ := doc["model_provider"].(string)
	mp = strings.TrimSpace(mp)
	if mp == "" || isReservedCodexProviderID(mp) {
		return qoderProviderID
	}
	return mp
}

// setTomlTopLevelKey 在 root table（第一个 [section] 之前）替换或新增一个 key=value 行。
// keyLine 是完整行如 `model_provider = "qccg"`。
// 不影响任何 [section] 块内的同名 key。
func setTomlTopLevelKey(text, key, keyLine string) string {
	firstSecRe := regexp.MustCompile(`(?m)^\s*\[`)
	firstSecLoc := firstSecRe.FindStringIndex(text)

	var rootText, sectionsText string
	if firstSecLoc == nil {
		rootText = text
		sectionsText = ""
	} else {
		rootText = text[:firstSecLoc[0]]
		sectionsText = text[firstSecLoc[0]:]
	}

	keyRe := regexp.MustCompile(fmt.Sprintf(`(?m)^\s*%s\s*=.*$`, regexp.QuoteMeta(key)))
	if keyRe.MatchString(rootText) {
		rootText = keyRe.ReplaceAllString(rootText, keyLine)
	} else {
		if rootText == "" {
			rootText = keyLine
		} else {
			rootText = strings.TrimRight(rootText, "\n") + "\n" + keyLine
		}
	}

	if rootText != "" && !strings.HasSuffix(rootText, "\n") {
		rootText += "\n"
	}
	return rootText + sectionsText
}

// removeTomlTopLevelKey 从 root table 中移除一个 key=value 行。
func removeTomlTopLevelKey(text, key string) string {
	firstSecRe := regexp.MustCompile(`(?m)^\s*\[`)
	firstSecLoc := firstSecRe.FindStringIndex(text)

	var rootText, sectionsText string
	if firstSecLoc == nil {
		rootText = text
		sectionsText = ""
	} else {
		rootText = text[:firstSecLoc[0]]
		sectionsText = text[firstSecLoc[0]:]
	}

	keyRe := regexp.MustCompile(fmt.Sprintf(`(?m)^\s*%s\s*=.*\n?`, regexp.QuoteMeta(key)))
	rootText = keyRe.ReplaceAllString(rootText, "")

	return strings.TrimRight(rootText, "\n") + sectionsText
}

// upsertTomlSection 替换或追加一个 TOML [section] 块。
// sectionName 形如 "model_providers.qccg"。
// sectionBody 是 section 内部的内容（不含 [header] 行）。
func upsertTomlSection(text, sectionName, sectionBody string) string {
	fullSection := "[" + sectionName + "]\n" + sectionBody

	headerPat := `(?m)^\[` + regexp.QuoteMeta(sectionName) + `\]\s*\n?`
	headerRe := regexp.MustCompile(headerPat)

	loc := headerRe.FindStringIndex(text)
	if loc == nil {
		// 不存在 → 追加到末尾。
		text = strings.TrimRight(text, "\n")
		if text != "" {
			text += "\n\n"
		}
		return text + fullSection + "\n"
	}

	// 找到该 section 的结束位置（下一个 [section] 或 EOF）。
	before := text[:loc[0]]
	afterHeader := text[loc[1]:]

	nextSecRe := regexp.MustCompile(`(?m)^\s*\[`)
	nextLoc := nextSecRe.FindStringIndex(afterHeader)

	if nextLoc == nil {
		return before + fullSection
	}
	return before + fullSection + "\n" + afterHeader[nextLoc[0]:]
}

// removeTomlSection 从 TOML 文本中移除一个 [section] 块。
func removeTomlSection(text, sectionName string) string {
	headerPat := `(?m)^\[` + regexp.QuoteMeta(sectionName) + `\]\s*\n?`
	headerRe := regexp.MustCompile(headerPat)

	loc := headerRe.FindStringIndex(text)
	if loc == nil {
		return text
	}

	before := text[:loc[0]]
	afterHeader := text[loc[1]:]

	nextSecRe := regexp.MustCompile(`(?m)^\s*\[`)
	nextLoc := nextSecRe.FindStringIndex(afterHeader)

	var result string
	if nextLoc == nil {
		result = strings.TrimRight(before, "\n")
	} else {
		result = before + afterHeader[nextLoc[0]:]
	}

	return result
}

// codexProviderSectionBody 返回 [model_providers.<id>] 内的 TOML 键值对。
func codexProviderSectionBody(port int) string {
	return fmt.Sprintf(`name = "QCCG"
base_url = "%s"
wire_api = "responses"
`, bridgeBaseURL(port)+codexProviderURL)
}

func writeCodexConfig(home string, port int, token, model string) error {
	dir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tomlPath := filepath.Join(dir, "config.toml")

	// 读取现有 config.toml 原始文本（保留注释、格式、其它 section）。
	origText := ""
	if data, err := os.ReadFile(tomlPath); err == nil {
		origText = string(data)
	}

	// 确定稳定的 provider ID（优先级：自定义 ID > "qccg"）。
	providerID := determineCodexProviderID(origText)

	// 字符串级编辑：只改目标字段，其余原文一字不动。
	newText := origText
	newText = setTomlTopLevelKey(newText, "model_provider",
		fmt.Sprintf("model_provider = %q", providerID))
	if model != "" {
		newText = setTomlTopLevelKey(newText, "model",
			fmt.Sprintf("model = %q", model))
	}
	newText = upsertTomlSection(newText,
		fmt.Sprintf("model_providers.%s", providerID),
		codexProviderSectionBody(port))

	// 语法校验（确保编辑后的 TOML 合法）。
	var tmp map[string]interface{}
	if err := toml.Unmarshal([]byte(newText), &tmp); err != nil {
		return fmt.Errorf("invalid TOML after edit: %w", err)
	}

	// 读取现有 auth.json 用于回滚。
	authPath := filepath.Join(dir, "auth.json")
	oldAuthExists := false
	var oldAuth []byte
	if data, err := os.ReadFile(authPath); err == nil {
		oldAuth = data
		oldAuthExists = true
	}

	// 第一阶段：写 auth.json。
	auth, err := readJSONObjectOrEmpty(authPath)
	if err != nil {
		return err
	}
	auth["OPENAI_API_KEY"] = token
	if err := writeJSONObjectOrdered(authPath, auth); err != nil {
		return err
	}

	// 第二阶段：写 config.toml（失败则回滚 auth.json）。
	if err := atomicWriteFile(tomlPath, []byte(newText), 0o644); err != nil {
		if oldAuthExists {
			_ = atomicWriteFile(authPath, oldAuth, 0o644)
		} else {
			_ = os.Remove(authPath)
		}
		return fmt.Errorf("write config.toml failed (auth rolled back): %w", err)
	}

	logger.Info("Wrote Codex config: %s (model=%s, provider=%s)", tomlPath, model, providerID)
	return nil
}

// cleanupCodexAuth removes QCCG's OPENAI_API_KEY from auth.json.
func cleanupCodexAuth(dir string) {
	authPath := filepath.Join(dir, "auth.json")
	auth, err := readJSONObjectOrEmpty(authPath)
	if err != nil {
		return
	}
	if v, ok := auth["OPENAI_API_KEY"].(string); ok && v == defaultQoderAPIKey {
		delete(auth, "OPENAI_API_KEY")
	}
	if len(auth) == 0 {
		_ = os.Remove(authPath)
	} else {
		_ = writeJSONObjectOrdered(authPath, auth)
	}
}

func removeCodexConfig(home string) error {
	dir := filepath.Join(home, ".codex")
	tomlPath := filepath.Join(dir, "config.toml")

	data, err := os.ReadFile(tomlPath)
	if err != nil {
		if os.IsNotExist(err) {
			cleanupCodexAuth(dir)
			logger.Info("Cleaned qccg fields from Codex config")
			return nil
		}
		return err
	}

	text := string(data)

	// 解析以判断我们写了哪个 provider ID。
	var doc map[string]interface{}
	if err := toml.Unmarshal(data, &doc); err != nil {
		// 无法解析 → 不碰文件，只清理 auth（以防万一）
		cleanupCodexAuth(dir)
		logger.Info("Cleaned qccg fields from Codex config")
		return nil
	}

	mp, _ := doc["model_provider"].(string)
	mp = strings.TrimSpace(mp)

	providerID := ""
	if mp == qoderProviderID {
		providerID = qoderProviderID
	} else if mp != "" && !isReservedCodexProviderID(mp) {
		// 检查 provider table 是否有 QCCG 签名（name = "QCCG"）。
		if providers, ok := doc["model_providers"].(map[string]interface{}); ok {
			if table, ok := providers[mp].(map[string]interface{}); ok {
				if name, ok := table["name"].(string); ok && name == "QCCG" {
					providerID = mp
				}
			}
		}
	}

	if providerID == "" {
		logger.Info("Codex config not managed by qccg, skipping removal")
		cleanupCodexAuth(dir)
		return nil
	}

	// 字符串级编辑：移除我们写入的字段。
	newText := text
	newText = removeTomlTopLevelKey(newText, "model_provider")
	newText = removeTomlTopLevelKey(newText, "model")
	newText = removeTomlSection(newText, fmt.Sprintf("model_providers.%s", providerID))
	newText = strings.TrimSpace(newText)

	if newText == "" {
		_ = os.Remove(tomlPath)
	} else {
		if err := atomicWriteFile(tomlPath, []byte(newText+"\n"), 0o644); err != nil {
			return err
		}
	}

	cleanupCodexAuth(dir)
	logger.Info("Cleaned qccg fields from Codex config")
	return nil
}

func readCodexStatus(home string) (bool, string, error) {
	if !hasBackupFile(home, "codex") {
		return false, "", nil
	}
	tomlPath := filepath.Join(home, ".codex", "config.toml")
	data, err := os.ReadFile(tomlPath)
	if err != nil {
		return true, "", nil
	}
	var doc map[string]interface{}
	if err := toml.Unmarshal(data, &doc); err != nil {
		return true, "", nil
	}
	model, _ := doc["model"].(string)
	return true, model, nil
}

// ===== Gemini CLI =====
//
// Gemini CLI 读 ~/.gemini/.env（一行一个 KEY=VALUE）+ ~/.gemini/settings.json。
// 我们写：
//
//	~/.gemini/.env:
//	  GOOGLE_GEMINI_BASE_URL=http://127.0.0.1:8963
//	  GEMINI_API_KEY=qccg
//	~/.gemini/settings.json:
//	  { "_qccg_managed": true, "selectedAuthType": "..." }   # 仅埋 marker
//
// .env 用 dotenv 风格的 KV 解析，保留用户其它 KEY 行；marker 进 settings.json
// 因为 .env 没有"嵌套字段"概念，无法保存 marker。

func writeGeminiConfig(home string, port int, token, model string) error {
	dir := filepath.Join(home, ".gemini")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	envPath := filepath.Join(dir, ".env")

	envMap, err := readDotEnv(envPath)
	if err != nil {
		return err
	}
	envMap["GOOGLE_GEMINI_BASE_URL"] = bridgeBaseURL(port)
	envMap["GEMINI_API_KEY"] = token
	if model != "" {
		envMap["GEMINI_MODEL"] = model
	}
	if err := writeDotEnv(envPath, envMap); err != nil {
		return err
	}
	logger.Info("Wrote Gemini config: %s (model=%s)", envPath, model)
	return nil
}

func removeGeminiConfig(home string) error {
	dir := filepath.Join(home, ".gemini")
	envPath := filepath.Join(dir, ".env")
	if envMap, err := readDotEnv(envPath); err == nil {
		delete(envMap, "GOOGLE_GEMINI_BASE_URL")
		delete(envMap, "GEMINI_API_KEY")
		delete(envMap, "GEMINI_MODEL")
		if len(envMap) == 0 {
			_ = os.Remove(envPath)
		} else {
			_ = writeDotEnv(envPath, envMap)
		}
	}
	logger.Info("Cleaned qccg fields from Gemini config")
	return nil
}

func readGeminiStatus(home string) (bool, string, error) {
	if !hasBackupFile(home, "gemini") {
		return false, "", nil
	}
	envMap, _ := readDotEnv(filepath.Join(home, ".gemini", ".env"))
	return true, envMap["GEMINI_MODEL"], nil
}

// ===== dotenv KV 解析 / 写回 =====
//
// 简单 dotenv：每行 KEY=VALUE，# 开头是注释。值如果含特殊字符就用双引号包；
// 不支持多行字符串、变量替换等扩展。

func readDotEnv(path string) (map[string]string, error) {
	out := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		k := strings.TrimSpace(line[:eq])
		v := strings.TrimSpace(line[eq+1:])
		// 去掉首尾引号
		if len(v) >= 2 {
			if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
				v = v[1 : len(v)-1]
			}
		}
		// 解码 percent-encoded（部分环境会写 URL 编码）
		if dec, err := url.QueryUnescape(v); err == nil {
			v = dec
		}
		out[k] = v
	}
	return out, nil
}

func writeDotEnv(path string, m map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("# Generated/maintained by QCCG\n")
	for _, k := range keys {
		v := m[k]
		// 含空格/引号/特殊字符 → 加双引号并转义
		if strings.ContainsAny(v, " \t\"'#=$") {
			v = strings.ReplaceAll(v, "\"", "\\\"")
			fmt.Fprintf(&b, "%s=\"%s\"\n", k, v)
		} else {
			fmt.Fprintf(&b, "%s=%s\n", k, v)
		}
	}
	return atomicWriteFile(path, []byte(b.String()), 0o644)
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
