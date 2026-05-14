package main

import "encoding/json"

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
		return bridgeDelta{}
	}
	inner, _ := wrapper["body"].(string)
	if inner == "" {
		return bridgeDelta{}
	}
	var innerJSON map[string]interface{}
	if err := json.Unmarshal([]byte(inner), &innerJSON); err != nil {
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
	return bridgeDelta{}
}
