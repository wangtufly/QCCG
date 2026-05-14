package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (b *bridge) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, err)
		return
	}
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		writeErr(w, err)
		return
	}
	stream, _ := req["stream"].(bool)
	model := strValDefault(req, "model", "lite")
	incomingMsgs, _ := req["messages"].([]interface{})
	tools := req["tools"]
	toolsEnabled := tools != nil

	prompt := extractLatestUserPrompt(incomingMsgs)
	messages := buildQoderMessages(b.templateMessages(), incomingMsgs, prompt, toolsEnabled)

	reqId := "chatcmpl-" + newRequestId()
	created := unixSec()
	ctx := r.Context()

	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(200)
		flusher, _ := w.(http.Flusher)

		var toolCallBuf []interface{}

		err = b.callQoder(ctx, messages, model, tools, func(d bridgeDelta) {
			chunk := makeChatChunk(reqId, created, model)
			choices := chunk["choices"].([]interface{})
			delta := choices[0].(map[string]interface{})["delta"].(map[string]interface{})
			if d.Content != "" {
				delta["role"] = "assistant"
				delta["content"] = d.Content
			}
			if d.ToolCalls != nil {
				delta["tool_calls"] = d.ToolCalls
				toolCallBuf = append(toolCallBuf, d.ToolCalls...)
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", string(data))
			if flusher != nil {
				flusher.Flush()
			}
		})
		if err != nil {
			errData, _ := json.Marshal(map[string]interface{}{
				"error": map[string]interface{}{"message": err.Error(), "type": "qoder_error"},
			})
			fmt.Fprintf(w, "data: %s\n\n", string(errData))
			if flusher != nil {
				flusher.Flush()
			}
		}
		finishReason := "stop"
		if len(toolCallBuf) > 0 {
			finishReason = "tool_calls"
		}
		done := makeChatChunk(reqId, created, model)
		choices := done["choices"].([]interface{})
		ch := choices[0].(map[string]interface{})
		ch["finish_reason"] = finishReason
		ch["delta"] = map[string]interface{}{}
		data, _ := json.Marshal(done)
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", string(data))
		if flusher != nil {
			flusher.Flush()
		}
	} else {
		var full strings.Builder
		var toolCallBuf []interface{}
		err = b.callQoder(ctx, messages, model, tools, func(d bridgeDelta) {
			if d.Content != "" {
				full.WriteString(d.Content)
			}
			if d.ToolCalls != nil {
				toolCallBuf = append(toolCallBuf, d.ToolCalls...)
			}
		})
		if err != nil {
			writeErr(w, fmt.Errorf("request failed: %w", err))
			return
		}
		finishReason := "stop"
		msg := map[string]interface{}{"role": "assistant", "content": full.String()}
		if len(toolCallBuf) > 0 {
			finishReason = "tool_calls"
			msg["tool_calls"] = toolCallBuf
			if full.Len() == 0 {
				msg["content"] = nil
			}
		}
		resp := map[string]interface{}{
			"id": reqId, "object": "chat.completion",
			"created": created, "model": model,
			"choices": []interface{}{
				map[string]interface{}{"index": 0, "message": msg, "finish_reason": finishReason},
			},
			"usage": map[string]interface{}{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
		}
		writeJSON(w, resp)
	}
}

func extractLatestUserPrompt(msgs []interface{}) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		m, _ := msgs[i].(map[string]interface{})
		if m["role"] == "user" {
			return normalizeMessageContent(m)
		}
	}
	return ""
}

func makeChatChunk(id string, created int64, model string) map[string]interface{} {
	return map[string]interface{}{
		"id": id, "object": "chat.completion.chunk",
		"created": created, "model": model,
		"choices": []interface{}{
			map[string]interface{}{"index": 0, "delta": map[string]interface{}{}, "finish_reason": nil},
		},
	}
}

func (b *bridge) templateMessages() []interface{} {
	if msgs, ok := b.templateBase["messages"].([]interface{}); ok {
		return msgs
	}
	return nil
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	data, _ := json.Marshal(v)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(data)
}

func writeErr(w http.ResponseWriter, err error) {
	body, _ := json.Marshal(map[string]interface{}{
		"error": map[string]interface{}{"message": err.Error(), "type": "qoder_error"},
	})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(500)
	w.Write(body)
}
