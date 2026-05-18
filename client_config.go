package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
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

// SaveClientConfigFile 保存编辑器内容回主配置文件，按 format 做语法校验。
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

func marshalJSONOrderPreserved(v interface{}) ([]byte, error) {
	switch obj := v.(type) {
	case map[string]interface{}:
		var b strings.Builder
		keySet := make(map[string]bool)
		keys := make([]string, 0, len(obj))
		for _, k := range topLevelOrder {
			if _, ok := obj[k]; ok {
				keys = append(keys, k)
				keySet[k] = true
			}
		}
		// 不在预定义顺序里的 key 按字母序追加
		extras := make([]string, 0)
		for k := range obj {
			if !keySet[k] {
				extras = append(extras, k)
			}
		}
		sort.Strings(extras)
		keys = append(keys, extras...)
		b.WriteString("{\n")
		for i, k := range keys {
			// 子对象用标准 MarshalIndent，再整体缩进
			valBytes, err := json.MarshalIndent(obj[k], "  ", "  ")
			if err != nil {
				return nil, err
			}
			comma := ","
			if i == len(keys)-1 {
				comma = ""
			}
			fmt.Fprintf(&b, "  %q: %s%s\n", k, valBytes, comma)
		}
		b.WriteString("}")
		return []byte(b.String()), nil
	default:
		return json.MarshalIndent(v, "", "  ")
	}
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
// 同样保留用户其它 model_providers 不动。

func codexProviderTable(port int) map[string]interface{} {
	return map[string]interface{}{
		"name":     "QCCG",
		"base_url": bridgeBaseURL(port) + codexProviderURL,
		"wire_api": "responses",
	}
}

func writeCodexConfig(home string, port int, token, model string) error {
	dir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tomlPath := filepath.Join(dir, "config.toml")

	// 读取原 config.toml（不存在则空）
	var doc map[string]interface{}
	if data, err := os.ReadFile(tomlPath); err == nil {
		if err := toml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("parse %s: %w", tomlPath, err)
		}
	}
	if doc == nil {
		doc = map[string]interface{}{}
	}

	doc["model_provider"] = qoderProviderID
	if model != "" {
		doc["model"] = model
	}
	providers, _ := doc["model_providers"].(map[string]interface{})
	if providers == nil {
		providers = map[string]interface{}{}
	}
	providers[qoderProviderID] = codexProviderTable(port)
	doc["model_providers"] = providers

	out, err := toml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal toml: %w", err)
	}
	if err := atomicWriteFile(tomlPath, out, 0o644); err != nil {
		return err
	}

	// 同步写 auth.json
	authPath := filepath.Join(dir, "auth.json")
	auth, _ := readJSONObjectOrEmpty(authPath)
	auth["OPENAI_API_KEY"] = token
	if err := writeJSONObjectOrdered(authPath, auth); err != nil {
		return err
	}

	logger.Info("Wrote Codex config: %s (model=%s)", tomlPath, model)
	return nil
}

func removeCodexConfig(home string) error {
	dir := filepath.Join(home, ".codex")
	tomlPath := filepath.Join(dir, "config.toml")

	var doc map[string]interface{}
	if data, err := os.ReadFile(tomlPath); err == nil {
		if err := toml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("parse %s: %w", tomlPath, err)
		}
	}
	if doc != nil {
		if v, ok := doc["model_provider"].(string); ok && v == qoderProviderID {
			delete(doc, "model_provider")
			delete(doc, "model")
		}
		if providers, ok := doc["model_providers"].(map[string]interface{}); ok {
			delete(providers, qoderProviderID)
			if len(providers) == 0 {
				delete(doc, "model_providers")
			} else {
				doc["model_providers"] = providers
			}
		}
		if len(doc) == 0 {
			_ = os.Remove(tomlPath)
		} else {
			out, err := toml.Marshal(doc)
			if err != nil {
				return err
			}
			if err := atomicWriteFile(tomlPath, out, 0o644); err != nil {
				return err
			}
		}
	}

	// auth.json: 只移除我们写的 OPENAI_API_KEY=qccg
	authPath := filepath.Join(dir, "auth.json")
	if auth, err := readJSONObjectOrEmpty(authPath); err == nil {
		if v, ok := auth["OPENAI_API_KEY"].(string); ok && v == defaultQoderAPIKey {
			delete(auth, "OPENAI_API_KEY")
		}
		if len(auth) == 0 {
			_ = os.Remove(authPath)
		} else {
			_ = writeJSONObjectOrdered(authPath, auth)
		}
	}
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
