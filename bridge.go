package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	_ "embed"
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

	fmt.Printf("[Bridge] Using token: %s (prefix: %s)\n", pat[:10]+"...", pat[:4])

	var identity authIdentity
	var name, id string

	// 判断 token 类型：dt- 开头是 device token，直接使用
	if strings.HasPrefix(pat, "dt-") {
		// OAuth device token：直接使用，不调用 exchangeJobToken
		fmt.Printf("[Bridge] Using OAuth device token directly\n")
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

	fmt.Printf("[bridge] session for %s (%s)\n", name, id)
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

// deepCopyMap does a JSON round-trip deep copy
func deepCopyMap(m map[string]interface{}) map[string]interface{} {
	data, _ := json.Marshal(m)
	var out map[string]interface{}
	json.Unmarshal(data, &out)
	return out
}

func (b *bridge) callQoder(ctx context.Context, messages []interface{}, model string, tools interface{}, onDelta func(bridgeDelta)) error {
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
	fmt.Printf("[bridge] prompt=%s\n", preview)

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
