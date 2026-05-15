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

	"qoder2api/logger"
)

// ClientConfig 是返回前端的客户端配置状态。
//   - ConfigPath/BaseURL/EnvVars 仅作展示
//   - Applied 表示「已经被 qoder2api 标注过」(由 Marker 字段判断，避免误判用户原本就有的配置)
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

// 内部 marker：用于识别「这一段配置是 qoder2api 写入的」，
// 移除时只移除自己写的部分，不动用户其它配置。
const (
	qoderMarkerKey   = "_qoder2api_managed"
	qoderProviderID  = "qoder2api"
	qoderAPIKey      = "qoder2api"
	codexProviderURL = "/v1"
)

func bridgeBaseURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

func (a *App) GetClientConfigs() []ClientConfig {
	port := a.bridgePort
	home, _ := os.UserHomeDir()

	configs := []ClientConfig{
		{
			Type:       "claude",
			Name:       "Claude Code",
			Icon:       "🧠",
			ConfigPath: filepath.Join(home, ".claude", "settings.json"),
			BaseURL:    bridgeBaseURL(port),
			EnvVars:    fmt.Sprintf(`export ANTHROPIC_BASE_URL="http://127.0.0.1:%d"`+"\n"+`export ANTHROPIC_AUTH_TOKEN="%s"`, port, qoderAPIKey),
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
			EnvVars:    fmt.Sprintf(`export GOOGLE_GEMINI_BASE_URL="http://127.0.0.1:%d"`+"\n"+`export GEMINI_API_KEY="%s"`, port, qoderAPIKey),
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
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	switch clientType {
	case "claude":
		return writeClaudeConfig(home, port, model)
	case "codex":
		return writeCodexConfig(home, port, model)
	case "gemini":
		return writeGeminiConfig(home, port, model)
	}
	return fmt.Errorf("unknown client type: %s", clientType)
}

// ClientConfigFile 是返回前端的「主配置文件原文 + 路径」结构，用于编辑器展示
type ClientConfigFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Format  string `json:"format"` // "json" / "toml" / "dotenv"
	Existed bool   `json:"existed"`
}

// ReadClientConfigFile 读取指定 client 主配置文件原文（不做任何解析/合并），
// 用于前端编辑器展示。若文件不存在返回空 content + Existed=false（不是错误）。
func (a *App) ReadClientConfigFile(clientType string) (*ClientConfigFile, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path, format := mainConfigPath(home, clientType)
	if path == "" {
		return nil, fmt.Errorf("unknown client type: %s", clientType)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ClientConfigFile{Path: path, Format: format, Existed: false}, nil
		}
		return nil, err
	}
	return &ClientConfigFile{Path: path, Content: string(data), Format: format, Existed: true}, nil
}

// SaveClientConfigFile 保存编辑器内容回主配置文件，按 format 做语法校验。
// .env 不校验（dotenv 没有官方语法）；JSON / TOML 校验失败直接返回 error，
// 不写入磁盘，避免破坏原文件。
func (a *App) SaveClientConfigFile(clientType, content string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path, format := mainConfigPath(home, clientType)
	if path == "" {
		return fmt.Errorf("unknown client type: %s", clientType)
	}
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
	if err := atomicWriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("写入失败: %w", err)
	}
	logger.Info("Saved %s config file: %s (%d bytes)", clientType, path, len(content))
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

func writeJSONObjectAtomic(path string, m map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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
// 我们只改 env 子树里我们关心的几个 key，并在顶层埋一个 _qoder2api_managed marker
// 用于识别状态；其它字段（hooks/permissions/MCP 等）原样保留。

func writeClaudeConfig(home string, port int, model string) error {
	path := filepath.Join(home, ".claude", "settings.json")
	root, err := readJSONObjectOrEmpty(path)
	if err != nil {
		return err
	}

	env, _ := root["env"].(map[string]interface{})
	if env == nil {
		env = map[string]interface{}{}
	}
	env["ANTHROPIC_BASE_URL"] = bridgeBaseURL(port)
	env["ANTHROPIC_AUTH_TOKEN"] = qoderAPIKey
	if model != "" {
		// 不强制写 SONNET/OPUS/HAIKU 三个槽位，避免覆盖用户的偏好。
		// 单一 model 由客户端自行选择，bridge 内部模型映射会兜底。
		env["ANTHROPIC_MODEL"] = model
	}
	root["env"] = env
	root[qoderMarkerKey] = true

	if err := writeJSONObjectAtomic(path, root); err != nil {
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
	delete(root, qoderMarkerKey)
	if len(root) == 0 {
		// 完全是我们写的，直接删文件
		_ = os.Remove(path)
		logger.Info("Removed Claude Code config: %s", path)
		return nil
	}
	if err := writeJSONObjectAtomic(path, root); err != nil {
		return err
	}
	logger.Info("Cleaned qoder2api fields from %s", path)
	return nil
}

func readClaudeStatus(home string) (bool, string, error) {
	path := filepath.Join(home, ".claude", "settings.json")
	root, err := readJSONObjectOrEmpty(path)
	if err != nil {
		return false, "", err
	}
	if _, ok := root[qoderMarkerKey]; !ok {
		return false, "", nil
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
//	model_provider = "qoder2api"
//	model = "gpt-5"  # 可选
//	[model_providers.qoder2api]
//	name = "Qoder2API"
//	base_url = "http://127.0.0.1:8963/v1"
//	wire_api = "responses"
//
// auth.json:
//
//	{ "OPENAI_API_KEY": "qoder2api" }
//
// 同样保留用户其它 model_providers 不动。

func codexProviderTable(port int) map[string]interface{} {
	return map[string]interface{}{
		"name":     "Qoder2API",
		"base_url": bridgeBaseURL(port) + codexProviderURL,
		"wire_api": "responses",
	}
}

func writeCodexConfig(home string, port int, model string) error {
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
	doc[qoderMarkerKey] = true

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
	auth["OPENAI_API_KEY"] = qoderAPIKey
	if err := writeJSONObjectAtomic(authPath, auth); err != nil {
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
		delete(doc, qoderMarkerKey)
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

	// auth.json: 只移除我们写的 OPENAI_API_KEY=qoder2api
	authPath := filepath.Join(dir, "auth.json")
	if auth, err := readJSONObjectOrEmpty(authPath); err == nil {
		if v, ok := auth["OPENAI_API_KEY"].(string); ok && v == qoderAPIKey {
			delete(auth, "OPENAI_API_KEY")
		}
		if len(auth) == 0 {
			_ = os.Remove(authPath)
		} else {
			_ = writeJSONObjectAtomic(authPath, auth)
		}
	}
	logger.Info("Cleaned qoder2api fields from Codex config")
	return nil
}

func readCodexStatus(home string) (bool, string, error) {
	tomlPath := filepath.Join(home, ".codex", "config.toml")
	data, err := os.ReadFile(tomlPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "", nil
		}
		return false, "", err
	}
	var doc map[string]interface{}
	if err := toml.Unmarshal(data, &doc); err != nil {
		return false, "", err
	}
	if _, ok := doc[qoderMarkerKey]; !ok {
		return false, "", nil
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
//	  GEMINI_API_KEY=qoder2api
//	~/.gemini/settings.json:
//	  { "_qoder2api_managed": true, "selectedAuthType": "..." }   # 仅埋 marker
//
// .env 用 dotenv 风格的 KV 解析，保留用户其它 KEY 行；marker 进 settings.json
// 因为 .env 没有"嵌套字段"概念，无法保存 marker。

func writeGeminiConfig(home string, port int, model string) error {
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
	envMap["GEMINI_API_KEY"] = qoderAPIKey
	if model != "" {
		envMap["GEMINI_MODEL"] = model
	}
	if err := writeDotEnv(envPath, envMap); err != nil {
		return err
	}

	// settings.json 只埋 marker
	settingsPath := filepath.Join(dir, "settings.json")
	settings, err := readJSONObjectOrEmpty(settingsPath)
	if err != nil {
		return err
	}
	settings[qoderMarkerKey] = true
	if err := writeJSONObjectAtomic(settingsPath, settings); err != nil {
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
	settingsPath := filepath.Join(dir, "settings.json")
	if settings, err := readJSONObjectOrEmpty(settingsPath); err == nil {
		delete(settings, qoderMarkerKey)
		if len(settings) == 0 {
			_ = os.Remove(settingsPath)
		} else {
			_ = writeJSONObjectAtomic(settingsPath, settings)
		}
	}
	logger.Info("Cleaned qoder2api fields from Gemini config")
	return nil
}

func readGeminiStatus(home string) (bool, string, error) {
	settingsPath := filepath.Join(home, ".gemini", "settings.json")
	settings, err := readJSONObjectOrEmpty(settingsPath)
	if err != nil {
		return false, "", err
	}
	if _, ok := settings[qoderMarkerKey]; !ok {
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
	b.WriteString("# Generated/maintained by Qoder2API\n")
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
