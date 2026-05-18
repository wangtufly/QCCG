package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	_ "embed"

	"qoder2api/account"
	"qoder2api/logger"
)

//go:embed baseprompt.json
var basePromptRaw []byte

func fetchUserInfoWithToken(token string) (map[string]interface{}, error) {
	req, _ := http.NewRequest("GET", "https://openapi.qoder.sh/api/v1/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	raw, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

type bridge struct {
	sess         *sessionContext
	client       *bearerClient
	templateBase map[string]interface{}
}

// newBridge 创建 API 转换桥接
// 支持两种认证方式：
// 1. OAuth device token (dt-xxx): 直接使用，调用 /api/v1/userinfo 获取用户信息
// 2. Personal Access Token (PAT): 调用 exchangeJobToken 转换为 session token
func newBridge(pat string) (*bridge, error) {
	mid := newUUID()
	mtoken := newBase64Token()
	mtype := newHexToken(18)

	logger.Info("Bridge using token: %s (prefix: %s)", pat[:10]+"...", pat[:4])

	var identity authIdentity
	var name, id string

	// 判断 token 类型：dt- 开头是 device token，直接使用
	if strings.HasPrefix(pat, "dt-") {
		// OAuth device token：直接使用，不调用 exchangeJobToken
		logger.Info("Bridge using OAuth device token directly")
		// 使用 device token 获取用户信息
		userInfo, err := fetchUserInfoWithToken(pat)
		if err != nil {
			return nil, fmt.Errorf("fetch user info: %w", err)
		}
		name = strVal(userInfo, "name")
		id = strVal(userInfo, "userId")
		identity = authIdentity{
			Name:               name,
			Aid:                id,
			Uid:                id,
			UserType:           strValDefault(userInfo, "userType", "personal_standard"),
			SecurityOauthToken: pat, // device token
			RefreshToken:       "",
		}
	} else {
		// PAT：调用 exchangeJobToken
		jt, err := exchangeJobToken(pat, mid, mtoken, mtype)
		if err != nil {
			return nil, fmt.Errorf("exchangeJobToken: %w", err)
		}
		name = strVal(jt, "name")
		id = strVal(jt, "id")
		identity = authIdentity{
			Name:               name,
			Aid:                id,
			Uid:                id,
			UserType:           strValDefault(jt, "userType", "personal_standard"),
			SecurityOauthToken: strVal(jt, "securityOauthToken"),
			RefreshToken:       strVal(jt, "refreshToken"),
		}
	}

	logger.Info("Bridge session for %s (%s)", name, id)
	sess, err := newSession(identity, mid, mtoken, mtype)
	if err != nil {
		return nil, err
	}
	client := newBearerClient(sess)

	// prepare template: replace placeholders
	tmpl := string(basePromptRaw)
	for _, ukey := range []string{"{UUID1}", "{UUID2}", "{UUID3}", "{UUID4}", "{UUID5}"} {
		tmpl = strings.ReplaceAll(tmpl, ukey, newUUID())
	}
	tmpl = strings.ReplaceAll(tmpl, "{TIME1}", fmt.Sprintf("%d", unixMs()))

	var templateBase map[string]interface{}
	if err := json.Unmarshal([]byte(tmpl), &templateBase); err != nil {
		return nil, fmt.Errorf("parse baseprompt: %w", err)
	}

	return &bridge{
		sess:         sess,
		client:       client,
		templateBase: templateBase,
	}, nil
}

// QoderModel 是返回给前端的精简模型条目（仅保留下拉选择必要字段）
type QoderModel struct {
	Key            string `json:"key"`
	DisplayName    string `json:"display_name"`
	Enable         bool   `json:"enable"`
	IsDefault      bool   `json:"is_default"`
	MaxInputTokens int    `json:"max_input_tokens,omitempty"`
}

// listAvailableModels 通过 cosy 签名调用 /algo/api/v2/model/list 拉取上游模型清单。
// 返回顶层 assistant 数组中 enable=true 的模型，按 is_default desc + display_name asc 排序。
func (b *bridge) listAvailableModels() ([]QoderModel, error) {
	const modelListURL = "https://api3.qoder.sh/algo/api/v2/model/list"
	resp, err := b.client.callGet(modelListURL)
	if err != nil {
		return nil, err
	}
	rawList, _ := resp["assistant"].([]interface{})
	out := make([]QoderModel, 0, len(rawList))
	for _, it := range rawList {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		enable, _ := m["enable"].(bool)
		if !enable {
			continue
		}
		out = append(out, QoderModel{
			Key:            strVal(m, "key"),
			DisplayName:    strVal(m, "display_name"),
			Enable:         enable,
			IsDefault:      func() bool { v, _ := m["is_default"].(bool); return v }(),
			MaxInputTokens: int(floatVal(m, "max_input_tokens")),
		})
	}
	return out, nil
}

// deepCopyMap does a JSON round-trip deep copy
func deepCopyMap(m map[string]interface{}) map[string]interface{} {
	data, _ := json.Marshal(m)
	var out map[string]interface{}
	json.Unmarshal(data, &out)
	return out
}

func (b *bridge) callQoder(ctx context.Context, agent string, messages []interface{}, model string, tools interface{}, onDelta func(bridgeDelta)) error {
	// 将客户端模型名（claude-sonnet-4-6 等）映射成 Qoder 上游内部 model.key（auto/ultimate/performance/lite/efficient）。
	// 上游对未知 key 会走兜底返回内容，但不会把这次调用计入 quota，这是「请求成功但 dashboard 无用量」的根因。
	originalModel := model
	model = mapModel(agent, model)
	if model != originalModel {
		logger.Debug("mapModel %s/%s -> %s", agent, originalModel, model)
	}
	body := deepCopyMap(b.templateBase)

	nid := newUUID()
	body["request_id"] = nid
	body["chat_record_id"] = nid
	body["request_set_id"] = newUUID()
	body["session_id"] = newUUID()
	body["stream"] = true
	body["aliyun_user_type"] = b.sess.Identity.UserType

	if mc, ok := body["model_config"].(map[string]interface{}); ok {
		mc["key"] = model
	}

	// Extract summary prompt from last user message for chat_context and business.name
	prompt := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if mm, ok := messages[i].(map[string]interface{}); ok && mm["role"] == "user" {
			prompt = normalizeMessageContent(mm)
			if prompt == "" {
				if contents, ok := mm["contents"].([]interface{}); ok {
					for _, block := range contents {
						if b, ok := block.(map[string]interface{}); ok {
							if t, ok := b["text"].(string); ok {
								prompt = t
								break
							}
						}
					}
				}
			}
			break
		}
	}

	if biz, ok := body["business"].(map[string]interface{}); ok {
		biz["id"] = newUUID()
		biz["begin_at"] = unixMs()
		if len(prompt) > 30 {
			biz["name"] = prompt[:30]
		} else {
			biz["name"] = prompt
		}
	}

	if cc, ok := body["chat_context"].(map[string]interface{}); ok {
		if txt, ok := cc["text"].(map[string]interface{}); ok {
			txt["text"] = prompt
		}
		if extra, ok := cc["extra"].(map[string]interface{}); ok {
			if oc, ok := extra["originalContent"].(map[string]interface{}); ok {
				oc["text"] = prompt
			}
		}
	}

	body["messages"] = messages
	if tools != nil {
		body["tools"] = tools
	}

	mcSource := "system"
	if mc, ok := body["model_config"].(map[string]interface{}); ok {
		if s, ok := mc["source"].(string); ok {
			mcSource = s
		}
	}

	qurl := "https://api3.qoder.sh/algo/api/v2/service/pro/sse/agent_chat_generation" +
		"?FetchKeys=llm_model_result&AgentId=agent_common&Encode=1"
	extra := map[string]string{
		"x-model-key":    model,
		"x-model-source": mcSource,
	}

	preview := prompt
	if len(preview) > 80 {
		preview = preview[:80] + "..."
	}
	logger.Info("callQoder model=%s prompt=%s", model, preview)
	logger.Debug("callQoder request body: %s", func() string { d, _ := json.Marshal(body); return string(d) }())

	var upstreamErr error
	streamErr := b.client.openStreamLines(ctx, qurl, body, extra, func(line string) {
		if !strings.HasPrefix(line, "data:") {
			return
		}
		dataPayload := strings.TrimSpace(line[5:])
		if dataPayload == "[DONE]" {
			return
		}
		delta := extractDelta(dataPayload)
		if delta.Err != nil {
			upstreamErr = delta.Err
			return
		}
		if !delta.isEmpty() {
			onDelta(delta)
		}
	})
	if upstreamErr != nil {
		return upstreamErr
	}
	return streamErr
}

// redactRequestBodyJSON 接收原始请求 JSON 字节，深拷贝后把可能含敏感对话内容的字段
// （messages[*].content / system / input / tools[*].description 等）的字符串值替换为
// `<redacted len=N>`，保留 JSON 结构和长度信息便于排查问题，但不泄露用户对话原文。
//
// 设计：
//  1. 只对 string 类型脱敏，结构和数字保留
//  2. 长度阈值：>32 才脱敏（避免把短角色名 "user" 也替换掉）
//  3. 按字段名递归：content/text/system/input/instructions/prompt/description
//  4. 失败时退回原始内容（脱敏只是日志辅助，不能影响主流程）
func redactRequestBodyJSON(raw []byte) string {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	redactValue(v, "")
	out, err := json.Marshal(v)
	if err != nil {
		return string(raw)
	}
	return string(out)
}

// sensitiveFieldNames 命中后其字符串子值会被脱敏（递归进数组/对象继续处理）
var sensitiveFieldNames = map[string]bool{
	"content":      true,
	"text":         true,
	"system":       true,
	"input":        true,
	"instructions": true,
	"prompt":       true,
	"description":  true,
}

func redactValue(v interface{}, parentField string) {
	switch x := v.(type) {
	case map[string]interface{}:
		for k, child := range x {
			if s, ok := child.(string); ok {
				if (sensitiveFieldNames[k] || sensitiveFieldNames[parentField]) && len(s) > 32 {
					x[k] = fmt.Sprintf("<redacted len=%d>", len(s))
				}
				continue
			}
			redactValue(child, k)
		}
	case []interface{}:
		for i, item := range x {
			if s, ok := item.(string); ok {
				if sensitiveFieldNames[parentField] && len(s) > 32 {
					x[i] = fmt.Sprintf("<redacted len=%d>", len(s))
				}
				continue
			}
			redactValue(item, parentField)
		}
	}
}

func strVal(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func strValDefault(m map[string]interface{}, key, def string) string {
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	return def
}

// inferAgent 根据模型名启发式推断 agent 类型，用于 chat/completions 这种多 agent 共用 endpoint
// 时选择正确的映射桶。模型名命中关键字按 gemini > claude > 默认 codex。
func inferAgent(model string) string {
	low := strings.ToLower(model)
	switch {
	case strings.Contains(low, "gemini"):
		return "gemini"
	case strings.Contains(low, "claude"), strings.Contains(low, "sonnet"), strings.Contains(low, "opus"), strings.Contains(low, "haiku"):
		return "claude"
	default:
		return "codex"
	}
}

// defaultModelMapping 是当用户未在 Settings.ModelMappings 中配置时，bridge 的内置兜底映射。
// 采用「家族关键字 → Qoder model.key」形式，依赖下面 mapModel 的双向 substring 模糊匹配，
// 一条 "sonnet" 即可覆盖 claude-sonnet-4-6 / claude-sonnet-4-20250514 等所有变体。
//
// 设计原则：默认表只负责让请求落到合法 SKU 不出错，差异化由用户在 UI 自行覆盖。
// Qoder 上游合法 key 仅 auto/ultimate/performance/lite/efficient（见 cmd/fetchmodels/models_result.json），
// GPT / Gemini 没有专属 SKU，统一映射到主力档 performance —— 既不浪费 ultimate 高 price_factor，
// 也不被 lite 限频；想分档（如 gpt-5→ultimate / gpt-5-mini→efficient）请在 UI 配置。
var defaultModelMapping = map[string]string{
	// Claude 三档
	"opus":   "ultimate",
	"sonnet": "performance",
	"haiku":  "lite",
	// 非 Claude 家族兜底
	"gpt":    "performance",
	"gemini": "performance",
}

// mapModel 解析顺序（参考 ccx 的 RedirectModel 算法）：
//  1. 用户 Settings.ModelMappings[agent] 精确命中
//  2. 用户 Settings.ModelMappings[agent] 双向 substring 模糊匹配（按 source 长度倒序，最长优先）
//  3. 用户旧 Settings.ModelMapping (deprecated 扁平表) 命中
//  4. defaultModelMapping 精确命中
//  5. defaultModelMapping 双向 substring 模糊匹配
//  6. ToLower 兜底（如客户端传 "Performance" 这种 display_name 大小写）
//
// agent 取值 "claude" / "codex" / "gemini"，由 handler 入口决定；空串表示未知 agent，跳过 agent 桶。
//
// 双向 substring 含义：source 包含 model（短关键字匹配长模型名，如 "sonnet" → "claude-sonnet-4-6"）
// 或 model 包含 source（长别名匹配短模型名，反向也能命中）。
func mapModel(agent, model string) string {
	if model == "" {
		return model
	}

	settings, _ := account.LoadSettings()

	// 1. agent 维度的用户配置优先
	if settings != nil && agent != "" {
		if table, ok := settings.ModelMappings[agent]; ok && len(table) > 0 {
			if mapped := lookupMapping(model, table); mapped != "" {
				if isValidQoderModelKey(mapped) {
					return mapped
				}
				logger.Error("invalid mapped model key in settings.model_mappings[%s]: %s -> %s, fallback to default mapping", agent, model, mapped)
			}
		}
	}

	// 2. 兼容旧的扁平 ModelMapping（仅当未配置 agent 桶时生效）
	if settings != nil && len(settings.ModelMapping) > 0 {
		if mapped := lookupMapping(model, settings.ModelMapping); mapped != "" {
			if isValidQoderModelKey(mapped) {
				return mapped
			}
			logger.Error("invalid mapped model key in deprecated settings.model_mapping: %s -> %s, fallback to default mapping", model, mapped)
		}
	}

	// 3. 内置默认映射兜底
	if mapped := lookupMapping(model, defaultModelMapping); mapped != "" {
		return mapped
	}

	// 4. 大小写归一化兜底（处理客户端直接传 "Performance" 这种）
	low := strings.ToLower(model)
	if isValidQoderModelKey(low) {
		return low
	}
	return "performance"
}

func isValidQoderModelKey(k string) bool {
	switch strings.ToLower(strings.TrimSpace(k)) {
	case "auto", "ultimate", "performance", "efficient", "lite":
		return true
	default:
		return false
	}
}

// lookupMapping 在单张映射表内执行：精确匹配 → 长度倒序 + 双向 substring 模糊匹配。
// 未命中返回空串。
func lookupMapping(model string, table map[string]string) string {
	if len(table) == 0 {
		return ""
	}
	// 1. 精确命中
	if v, ok := table[model]; ok && v != "" {
		return v
	}
	// 2. 长度倒序，确保 "claude-sonnet-4-6" 优先于 "sonnet" 命中
	type kv struct{ src, dst string }
	pairs := make([]kv, 0, len(table))
	for k, v := range table {
		if k == "" || v == "" {
			continue
		}
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool { return len(pairs[i].src) > len(pairs[j].src) })
	mLow := strings.ToLower(model)
	for _, p := range pairs {
		sLow := strings.ToLower(p.src)
		if strings.Contains(mLow, sLow) || strings.Contains(sLow, mLow) {
			return p.dst
		}
	}
	return ""
}
