package main

import "testing"

func TestBuildQoderMessages_SingleUser(t *testing.T) {
	incoming := []interface{}{
		map[string]interface{}{"role": "user", "content": "hello"},
	}
	templateMsgs := []interface{}{
		map[string]interface{}{"role": "system", "content": "You are helpful."},
	}
	result := buildQoderMessages(templateMsgs, incoming, "hello", false)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	first := result[0].(map[string]interface{})
	if first["role"] != "system" {
		t.Fatalf("expected system, got %s", first["role"])
	}
	last := result[len(result)-1].(map[string]interface{})
	if last["role"] != "user" {
		t.Fatalf("expected user, got %s", last["role"])
	}
}

func TestBuildQoderMessages_WithToolCalls(t *testing.T) {
	incoming := []interface{}{
		map[string]interface{}{"role": "user", "content": "read file"},
		map[string]interface{}{
			"role": "assistant", "content": "",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"id": "call_1", "type": "function",
					"function": map[string]interface{}{"name": "read", "arguments": `{"path":"x"}`},
				},
			},
		},
		map[string]interface{}{"role": "tool", "tool_call_id": "call_1", "content": "file content"},
		map[string]interface{}{"role": "user", "content": "now fix it"},
	}
	result := buildQoderMessages(nil, incoming, "now fix it", true)
	if len(result) < 4 {
		t.Fatalf("expected >= 4 messages, got %d", len(result))
	}
}

func TestBuildQoderMessages_IncomingSystemOverridesTemplate(t *testing.T) {
	incoming := []interface{}{
		map[string]interface{}{"role": "system", "content": "Custom system"},
		map[string]interface{}{"role": "user", "content": "hi"},
	}
	templateMsgs := []interface{}{
		map[string]interface{}{"role": "system", "content": "Template system"},
	}
	result := buildQoderMessages(templateMsgs, incoming, "hi", false)
	// Should NOT include template system when incoming has its own
	for _, m := range result {
		mm := m.(map[string]interface{})
		if mm["role"] == "system" && mm["content"] == "Template system" {
			t.Fatal("template system should not be included when incoming has system")
		}
	}
}

func TestNormalizeMessageContent_String(t *testing.T) {
	msg := map[string]interface{}{"content": "hello"}
	if got := normalizeMessageContent(msg); got != "hello" {
		t.Fatalf("expected 'hello', got '%s'", got)
	}
}

func TestNormalizeMessageContent_Array(t *testing.T) {
	msg := map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{"type": "text", "text": "part1"},
			map[string]interface{}{"type": "text", "text": "part2"},
		},
	}
	got := normalizeMessageContent(msg)
	if got != "part1\n\npart2" {
		t.Fatalf("expected 'part1\\n\\npart2', got '%s'", got)
	}
}

// ===== mapModel / lookupMapping =====
// 验证模糊匹配算法：精确 → 长度倒序 → 双向 substring → ToLower 兜底。
// 参考 ccx/backend-go/internal/config/config_utils.go::RedirectModel。

func TestLookupMapping_ExactMatch(t *testing.T) {
	table := map[string]string{
		"claude-sonnet-4-6": "ultimate",
		"sonnet":            "performance",
	}
	if got := lookupMapping("claude-sonnet-4-6", table); got != "ultimate" {
		t.Fatalf("expected 'ultimate' (exact wins), got '%s'", got)
	}
}

func TestLookupMapping_LongestSourceWins(t *testing.T) {
	// 长 source 应优先于短关键字，避免 sonnet 抢走 claude-sonnet-4-6 的命中
	table := map[string]string{
		"sonnet":            "performance",
		"claude-sonnet-4-6": "ultimate",
	}
	if got := lookupMapping("claude-sonnet-4-6-20250929", table); got != "ultimate" {
		t.Fatalf("expected 'ultimate' (longest source wins), got '%s'", got)
	}
}

func TestLookupMapping_ShortSourceFuzzy(t *testing.T) {
	// 短关键字命中长模型名（cc-switch 风格）
	table := map[string]string{"sonnet": "performance"}
	if got := lookupMapping("claude-sonnet-4-6-20250929", table); got != "performance" {
		t.Fatalf("expected 'performance' (sonnet substring match), got '%s'", got)
	}
}

func TestLookupMapping_ReverseSubstring(t *testing.T) {
	// model 是 source 的子串：长别名也能命中短客户端模型名
	table := map[string]string{"claude-sonnet-4-6": "ultimate"}
	if got := lookupMapping("sonnet", table); got != "ultimate" {
		t.Fatalf("expected 'ultimate' (model substring of source), got '%s'", got)
	}
}

func TestLookupMapping_CaseInsensitive(t *testing.T) {
	table := map[string]string{"sonnet": "performance"}
	if got := lookupMapping("Claude-SONNET-4-6", table); got != "performance" {
		t.Fatalf("expected case-insensitive fuzzy match, got '%s'", got)
	}
}

func TestLookupMapping_NoMatch(t *testing.T) {
	table := map[string]string{"sonnet": "performance"}
	if got := lookupMapping("gemini-2.5-pro", table); got != "" {
		t.Fatalf("expected empty (no match), got '%s'", got)
	}
}

func TestLookupMapping_EmptyTable(t *testing.T) {
	if got := lookupMapping("anything", nil); got != "" {
		t.Fatalf("expected empty for nil table, got '%s'", got)
	}
}

func TestLookupMapping_SkipsEmptyEntries(t *testing.T) {
	table := map[string]string{"": "ultimate", "sonnet": ""}
	if got := lookupMapping("claude-sonnet-4-6", table); got != "" {
		t.Fatalf("expected empty entries to be skipped, got '%s'", got)
	}
}

func TestMapModel_DefaultMappingFallback(t *testing.T) {
	// 直接验证内置 defaultModelMapping 中的关键字均能通过 lookupMapping 命中长模型名。
	// 不调 mapModel() 是为了避开 ~/.qoder2api/settings.json 可能存在的用户覆盖，
	// 让单测在任意环境（含已配置过映射的开发机）都稳定通过。
	cases := map[string]string{
		// Claude 三档
		"claude-sonnet-4-6":         "performance",
		"claude-opus-4-7-1m":        "ultimate",
		"claude-haiku-4-5-20251001": "lite",

		// GPT 家族统一兜底到 performance（不细分 mini/nano，由用户在 UI 自配）
		"gpt-5":         "performance",
		"gpt-5-mini":    "performance",
		"gpt-5-nano":    "performance",
		"gpt-4o":        "performance",
		"gpt-4o-mini":   "performance",
		"gpt-4.1":       "performance",
		"gpt-3.5-turbo": "performance",

		// Gemini 家族统一兜底到 performance
		"gemini-2.5-pro":        "performance",
		"gemini-2.5-flash":      "performance",
		"gemini-2.5-flash-lite": "performance",
		"gemini-1.5-pro-latest": "performance",
	}
	for in, want := range cases {
		if got := lookupMapping(in, defaultModelMapping); got != want {
			t.Errorf("default mapping %q: want %q, got %q", in, want, got)
		}
	}

	// o-series 推理模型不在默认表（避免与 OpenAI gpt 家族混淆），应未命中
	if got := lookupMapping("o3", defaultModelMapping); got != "" {
		t.Errorf("o-series should not be in default mapping, got %q", got)
	}
}

func TestMapModel_ToLowerFallback(t *testing.T) {
	// 完全没命中时归一化大小写，避免 "ZZZNotARealModel" 这种直传上游 500。
	// 用一个绝不会出现在 defaultModelMapping / 用户配置中的字符串，保证测试稳定。
	if got := mapModel("ZZZNotARealModel"); got != "zzznotarealmodel" {
		t.Fatalf("expected ToLower fallback, got '%s'", got)
	}
}

func TestMapModel_EmptyPassthrough(t *testing.T) {
	if got := mapModel(""); got != "" {
		t.Fatalf("expected empty passthrough, got '%s'", got)
	}
}
