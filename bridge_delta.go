package main

import (
	"encoding/json"
	"fmt"

	"qoder2api/logger"
)

type bridgeDelta struct {
	Role         string
	Content      string
	ToolCalls    []interface{}
	InputTokens  int
	OutputTokens int
	Err          error // 上游返回业务错误时非 nil
}

func (d bridgeDelta) isEmpty() bool {
	return d.Role == "" && d.Content == "" && d.ToolCalls == nil && d.InputTokens == 0 && d.OutputTokens == 0 && d.Err == nil
}

func extractDelta(dataLine string) bridgeDelta {
	var wrapper map[string]interface{}
	if err := json.Unmarshal([]byte(dataLine), &wrapper); err != nil {
		logger.Debug("[delta] unmarshal wrapper failed: %v, raw=%s", err, dataLine)
		return bridgeDelta{}
	}
	inner, _ := wrapper["body"].(string)
	if inner == "" {
		logger.Debug("[delta] no body field, keys=%v, raw=%s", mapKeys(wrapper), dataLine)
		return bridgeDelta{}
	}
	var innerJSON map[string]interface{}
	if err := json.Unmarshal([]byte(inner), &innerJSON); err != nil {
		logger.Debug("[delta] unmarshal inner failed: %v", err)
		return bridgeDelta{}
	}
	choices, _ := innerJSON["choices"].([]interface{})
	for _, ch := range choices {
		chMap, _ := ch.(map[string]interface{})
		delta, _ := chMap["delta"].(map[string]interface{})
		if delta == nil {
			continue
		}
		role, _ := delta["role"].(string)
		content, _ := delta["content"].(string)
		var toolCalls []interface{}
		if tc, ok := delta["tool_calls"].([]interface{}); ok && len(tc) > 0 {
			toolCalls = tc
		}
		if role != "" || content != "" || toolCalls != nil {
			return bridgeDelta{Role: role, Content: content, ToolCalls: toolCalls}
		}
	}
	// 上游最后一个 chunk 通常 choices 为空，但顶层携带 usage 统计
	if usage, ok := innerJSON["usage"].(map[string]interface{}); ok {
		in := int(floatVal(usage, "prompt_tokens"))
		out := int(floatVal(usage, "completion_tokens"))
		if in > 0 || out > 0 {
			return bridgeDelta{InputTokens: in, OutputTokens: out}
		}
	}
	// 上游业务错误：{"code":"115","message":"..."}
	if code, ok := innerJSON["code"].(string); ok && code != "" && code != "0" {
		msg, _ := innerJSON["message"].(string)
		err := fmt.Errorf("upstream error code=%s: %s", code, msg)
		logger.Error("[delta] %v", err)
		return bridgeDelta{Err: err}
	}
	logger.Debug("[delta] no valid choices found, inner=%s", inner)
	return bridgeDelta{}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func floatVal(m map[string]interface{}, key string) float64 {
	v, _ := m[key].(float64)
	return v
}
