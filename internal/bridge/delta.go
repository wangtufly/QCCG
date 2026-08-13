package bridge

import (
	"encoding/json"
	"fmt"
	"time"

	"qccg/internal/cosy"
	"qccg/logger"
)

type Delta struct {
	Role         string
	Content      string
	Reasoning    string // 上游推理过程（reasoning_content）
	ToolCalls    []interface{}
	InputTokens  int
	OutputTokens int
	Err          error         // 上游返回业务错误时非 nil
	IsRetryable  bool          // 是否为可重试的临时错误（排队、限流等）
	RetryAfter   time.Duration // 上游建议的重试等待时间，0 表示使用本地退避
}

type upstreamError struct {
	err       error
	retryable bool
}

func (e *upstreamError) Error() string { return e.err.Error() }
func (e *upstreamError) Unwrap() error { return e.err }

func newUpstreamError(err error, retryable bool) error {
	return &upstreamError{err: err, retryable: retryable}
}

func (d Delta) isEmpty() bool {
	return d.Role == "" && d.Content == "" && d.Reasoning == "" && d.ToolCalls == nil && d.InputTokens == 0 && d.OutputTokens == 0 && d.Err == nil && !d.IsRetryable && d.RetryAfter == 0
}

func ExtractDelta(dataLine string) Delta {
	var wrapper map[string]interface{}
	if err := json.Unmarshal([]byte(dataLine), &wrapper); err != nil {
		logger.Debug("[delta] unmarshal wrapper failed: %v, raw=%s", err, dataLine)
		return Delta{}
	}
	inner, _ := wrapper["body"].(string)
	if inner == "" {
		logger.Debug("[delta] no body field, keys=%v, raw=%s", mapKeys(wrapper), dataLine)
		return Delta{}
	}
	var innerJSON map[string]interface{}
	if err := json.Unmarshal([]byte(inner), &innerJSON); err != nil {
		logger.Debug("[delta] unmarshal inner failed: %v", err)
		return Delta{}
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
		reasoning, _ := delta["reasoning_content"].(string)
		var toolCalls []interface{}
		if tc, ok := delta["tool_calls"].([]interface{}); ok && len(tc) > 0 {
			toolCalls = tc
		}
		if role != "" || content != "" || reasoning != "" || toolCalls != nil {
			return Delta{Role: role, Content: content, Reasoning: reasoning, ToolCalls: toolCalls}
		}
	}
	// 上游最后一个 chunk 通常 choices 为空，但顶层携带 usage 统计
	if usage, ok := innerJSON["usage"].(map[string]interface{}); ok {
		in := int(cosy.FloatVal(usage, "prompt_tokens"))
		out := int(cosy.FloatVal(usage, "completion_tokens"))
		if in > 0 || out > 0 {
			return Delta{InputTokens: in, OutputTokens: out}
		}
	}
	// 上游业务错误有两种形式：直接返回业务码，或外层 403 的 message 内嵌业务码。
	if code, ok := innerJSON["code"].(string); ok && code != "" && code != "0" {
		msg, _ := innerJSON["message"].(string)
		return parseUpstreamError(code, msg)
	}
	logger.Debug("[delta] no valid choices found, inner=%s", inner)
	return Delta{}
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

// parseUpstreamError 解析上游业务错误。Qoder 会把 10605 包在外层 403 的 message 中。
func parseUpstreamError(code, msg string) Delta {
	for range 2 {
		var envelope struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal([]byte(msg), &envelope); err != nil || envelope.Code == "" {
			break
		}
		code, msg = envelope.Code, envelope.Message
	}

	if code == "10605" {
		return parseQueueStatus(code, msg)
	}

	retryableCodes := map[string]bool{
		"429": true,
		"503": true,
	}
	err := fmt.Errorf("upstream error code=%s: %s", code, msg)
	logger.Error("[delta] %v", err)
	return Delta{Err: newUpstreamError(err, retryableCodes[code]), IsRetryable: retryableCodes[code]}
}

// parseQueueStatus 解析错误码 10605 的排队状态信息
func parseQueueStatus(code, msg string) Delta {
	var queueInfo map[string]interface{}
	if err := json.Unmarshal([]byte(msg), &queueInfo); err != nil {
		logger.Error("[delta] code=%s, failed to parse queue info: %v, msg=%s", code, err, msg)
		return Delta{Err: newUpstreamError(fmt.Errorf("upstream error code=%s: %s", code, msg), true), IsRetryable: true}
	}

	isQueued, _ := queueInfo["isQueued"].(bool)
	modelKey, _ := queueInfo["modelKey"].(string)
	queueCount := int(cosy.FloatVal(queueInfo, "queueCount"))
	queueType, _ := queueInfo["queueType"].(string)
	serviceAvailable, _ := queueInfo["serviceAvailable"].(bool)
	waitTime := time.Duration(cosy.FloatVal(queueInfo, "waitTime")) * time.Millisecond
	if waitTime <= 0 {
		waitTime = time.Duration(cosy.FloatVal(queueInfo, "retry_after_ms")) * time.Millisecond
	}
	if waitTime <= 0 {
		waitTime = time.Duration(cosy.FloatVal(queueInfo, "retryAfterMs")) * time.Millisecond
	}
	if waitTime <= 0 {
		waitTime = time.Duration(cosy.FloatVal(queueInfo, "retryAfterSeconds")) * time.Second
	}

	logger.Info("[delta] queue status code=%s: isQueued=%v model=%s count=%d type=%s available=%v wait=%s",
		code, isQueued, modelKey, queueCount, queueType, serviceAvailable, waitTime)

	if serviceAvailable && !isQueued {
		return Delta{Err: newUpstreamError(fmt.Errorf("temporarily throttled (code=%s)", code), true), IsRetryable: true, RetryAfter: waitTime}
	}
	if isQueued {
		errMsg := fmt.Sprintf("request queued: model=%s, position=%d, type=%s, wait=%s", modelKey, queueCount, queueType, waitTime)
		return Delta{Err: newUpstreamError(fmt.Errorf("%s", errMsg), true), IsRetryable: true, RetryAfter: waitTime}
	}
	return Delta{Err: newUpstreamError(fmt.Errorf("upstream error code=%s: %s", code, msg), true), IsRetryable: true, RetryAfter: waitTime}
}
