package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (b *bridge) handleClaudeMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeClaudeErr(w, err)
		return
	}
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		writeClaudeErr(w, err)
		return
	}
	stream, _ := req["stream"].(bool)
	model := strValDefault(req, "model", "lite")
	incomingMsgs, _ := req["messages"].([]interface{})

	// Claude format: tools use name+input_schema directly
	// Convert to OpenAI-style for Qoder upstream, or pass through
	tools := req["tools"]
	toolsEnabled := tools != nil

	// If there's a system field, prepend it as a system message
	if sys := req["system"]; sys != nil {
		sysText := ""
		switch v := sys.(type) {
		case string:
			sysText = v
		case []interface{}:
			for _, block := range v {
				if bk, ok := block.(map[string]interface{}); ok {
					if t, ok := bk["text"].(string); ok {
						if sysText != "" {
							sysText += "\n"
						}
						sysText += t
					}
				}
			}
		}
		if sysText != "" {
			sysMsg := map[string]interface{}{"role": "system", "content": sysText}
			incomingMsgs = append([]interface{}{sysMsg}, incomingMsgs...)
		}
	}

	prompt := extractLatestUserPrompt(incomingMsgs)
	messages := buildQoderMessages(b.templateMessages(), incomingMsgs, prompt, toolsEnabled)

	msgId := "msg_" + newRequestId()
	ctx := r.Context()

	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(200)
		flusher, _ := w.(http.Flusher)

		writeSse := func(eventType string, data interface{}) {
			d, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(d))
			if flusher != nil {
				flusher.Flush()
			}
		}

		writeSse("message_start", map[string]interface{}{
			"type": "message_start",
			"message": map[string]interface{}{
				"id": msgId, "type": "message", "role": "assistant", "model": model,
				"stop_reason": nil, "stop_sequence": nil,
				"content": []interface{}{},
				"usage":   map[string]interface{}{"input_tokens": 0, "output_tokens": 0},
			},
		})
		writeSse("content_block_start", map[string]interface{}{
			"type": "content_block_start", "index": 0,
			"content_block": map[string]interface{}{"type": "text", "text": ""},
		})
		writeSse("ping", map[string]interface{}{"type": "ping"})

		var toolCallBuf []interface{}
		contentBlockIndex := 0

		err = b.callQoder(ctx, messages, model, tools, func(d bridgeDelta) {
			if d.Content != "" {
				writeSse("content_block_delta", map[string]interface{}{
					"type": "content_block_delta", "index": contentBlockIndex,
					"delta": map[string]interface{}{"type": "text_delta", "text": d.Content},
				})
			}
			if d.ToolCalls != nil {
				toolCallBuf = append(toolCallBuf, d.ToolCalls...)
			}
		})
		if err != nil {
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
			if flusher != nil {
				flusher.Flush()
			}
		}

		// Close text block
		writeSse("content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": contentBlockIndex})

		// If there were tool calls, emit them as tool_use blocks
		for i, tc := range toolCallBuf {
			tcMap, _ := tc.(map[string]interface{})
			if tcMap == nil {
				continue
			}
			blockIdx := contentBlockIndex + 1 + i
			fn, _ := tcMap["function"].(map[string]interface{})
			toolId, _ := tcMap["id"].(string)
			if toolId == "" {
				toolId = "toolu_" + newRequestId()
			}
			name, _ := fn["name"].(string)
			argsStr, _ := fn["arguments"].(string)
			var input interface{}
			json.Unmarshal([]byte(argsStr), &input)
			if input == nil {
				input = map[string]interface{}{}
			}

			writeSse("content_block_start", map[string]interface{}{
				"type": "content_block_start", "index": blockIdx,
				"content_block": map[string]interface{}{
					"type": "tool_use", "id": toolId, "name": name, "input": map[string]interface{}{},
				},
			})
			writeSse("content_block_delta", map[string]interface{}{
				"type": "content_block_delta", "index": blockIdx,
				"delta": map[string]interface{}{"type": "input_json_delta", "partial_json": argsStr},
			})
			writeSse("content_block_stop", map[string]interface{}{"type": "content_block_stop", "index": blockIdx})
		}

		stopReason := "end_turn"
		if len(toolCallBuf) > 0 {
			stopReason = "tool_use"
		}

		writeSse("message_delta", map[string]interface{}{
			"type":  "message_delta",
			"delta": map[string]interface{}{"stop_reason": stopReason, "stop_sequence": nil},
			"usage": map[string]interface{}{"output_tokens": 0},
		})
		writeSse("message_stop", map[string]interface{}{"type": "message_stop"})
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
			writeClaudeErr(w, fmt.Errorf("request failed: %w", err))
			return
		}

		content := []interface{}{}
		if full.Len() > 0 {
			content = append(content, map[string]interface{}{"type": "text", "text": full.String()})
		}
		for _, tc := range toolCallBuf {
			tcMap, _ := tc.(map[string]interface{})
			if tcMap == nil {
				continue
			}
			fn, _ := tcMap["function"].(map[string]interface{})
			toolId, _ := tcMap["id"].(string)
			if toolId == "" {
				toolId = "toolu_" + newRequestId()
			}
			name, _ := fn["name"].(string)
			argsStr, _ := fn["arguments"].(string)
			var input interface{}
			json.Unmarshal([]byte(argsStr), &input)
			if input == nil {
				input = map[string]interface{}{}
			}
			content = append(content, map[string]interface{}{
				"type": "tool_use", "id": toolId, "name": name, "input": input,
			})
		}

		stopReason := "end_turn"
		if len(toolCallBuf) > 0 {
			stopReason = "tool_use"
		}

		resp := map[string]interface{}{
			"id": msgId, "type": "message", "role": "assistant", "model": model,
			"stop_reason": stopReason, "stop_sequence": nil,
			"content": content,
			"usage":   map[string]interface{}{"input_tokens": 0, "output_tokens": 0},
		}
		writeJSON(w, resp)
	}
}

func writeClaudeErr(w http.ResponseWriter, err error) {
	body, _ := json.Marshal(map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    "api_error",
			"message": err.Error(),
		},
	})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(500)
	w.Write(body)
}
