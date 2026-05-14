package main

import (
	"encoding/json"

	"qoder2api/logger"
)

type bridgeDelta struct {
	Role      string
	Content   string
	ToolCalls []interface{}
}

func (d bridgeDelta) isEmpty() bool {
	return d.Role == "" && d.Content == "" && d.ToolCalls == nil
}

func extractDelta(dataLine string) bridgeDelta {
	var wrapper map[string]interface{}
	if err := json.Unmarshal([]byte(dataLine), &wrapper); err != nil {
		logger.Debug("[delta] unmarshal wrapper failed: %v, raw=%s", err, truncate(dataLine, 200))
		return bridgeDelta{}
	}
	inner, _ := wrapper["body"].(string)
	if inner == "" {
		logger.Debug("[delta] no body field, keys=%v, raw=%s", mapKeys(wrapper), truncate(dataLine, 200))
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
	logger.Debug("[delta] no valid choices found, inner=%s", truncate(inner, 300))
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
