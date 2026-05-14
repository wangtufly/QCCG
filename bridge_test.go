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
