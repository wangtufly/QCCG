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
	Key         string `json:"key"`
	DisplayName string `json:"display_name"`
	Enable      bool   `json:"enable"`
	IsDefault   bool   `json:"is_default"`
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
			Key:         strVal(m, "key"),
			DisplayName: strVal(m, "display_name"),
			Enable:      enable,
			IsDefault:   func() bool { v, _ := m["is_default"].(bool); return v }(),
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

func (b *bridge) callQoder(ctx context.Context, messages []interface{}, model string, tools interface{}, onDelta func(bridgeDelta)) error {
	// 将客户端模型名（claude-sonnet-4-6 等）映射成 Qoder 上游内部 model.key（auto/ultimate/performance/lite/efficient）。
	// 上游对未知 key 会走兜底返回内容，但不会把这次调用计入 quota，这是「请求成功但 dashboard 无用量」的根因。
	originalModel := model
	model = mapModel(model)
	if model != originalModel {
		logger.Debug("mapModel %s -> %s", originalModel, model)
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

	return b.client.openStreamLines(ctx, qurl, body, extra, func(line string) {
		if !strings.HasPrefix(line, "data:") {
			return
		}
		dataPayload := strings.TrimSpace(line[5:])
		if dataPayload == "[DONE]" {
			return
		}
		delta := extractDelta(dataPayload)
		if !delta.isEmpty() {
			onDelta(delta)
		}
	})
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

// defaultModelMapping 是当用户未在 Settings.ModelMapping 中配置时，bridge 的内置兜底映射。
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
//  1. 用户 Settings.ModelMapping 精确命中
//  2. 用户 Settings.ModelMapping 双向 substring 模糊匹配（按 source 长度倒序，最长优先）
//  3. defaultModelMapping 精确命中
//  4. defaultModelMapping 双向 substring 模糊匹配
//  5. ToLower 兜底（如客户端传 "Performance" 这种 display_name 大小写）
//
// 双向 substring 含义：source 包含 model（短关键字匹配长模型名，如 "sonnet" → "claude-sonnet-4-6"）
// 或 model 包含 source（长别名匹配短模型名，反向也能命中）。
func mapModel(model string) string {
	if model == "" {
		return model
	}

	// 用户配置优先
	settings, err := account.LoadSettings()
	if err == nil && settings != nil && len(settings.ModelMapping) > 0 {
		if mapped := lookupMapping(model, settings.ModelMapping); mapped != "" {
			return mapped
		}
	}

	// 内置默认映射兜底
	if mapped := lookupMapping(model, defaultModelMapping); mapped != "" {
		return mapped
	}

	// 大小写归一化兜底（处理客户端直接传 "Performance" 这种）
	return strings.ToLower(model)
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
