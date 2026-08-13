package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"qccg/internal/cosy"
)

func TestParseQueueStatus(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		msg         string
		wantErr     bool
		wantRetry   bool
		errContains string
	}{
		{
			name:        "临时限流-服务可用",
			code:        "10605",
			msg:         `{"isQueued":false,"modelKey":"dmodel","queueCount":0,"queueType":"slow","retryAfterSeconds":0,"serviceAvailable":true,"waitTime":0}`,
			wantErr:     true,
			wantRetry:   true,
			errContains: "temporarily throttled",
		},
		{
			name:        "排队中",
			code:        "10605",
			msg:         `{"isQueued":true,"modelKey":"dmodel","queueCount":5,"queueType":"normal","retryAfterSeconds":10,"serviceAvailable":true,"waitTime":2000}`,
			wantErr:     true,
			wantRetry:   true,
			errContains: "request queued",
		},
		{
			name:        "非JSON消息",
			code:        "10605",
			msg:         "simple error message",
			wantErr:     true,
			wantRetry:   true,
			errContains: "upstream error code=10605",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delta := parseQueueStatus(tt.code, tt.msg)

			if (delta.Err != nil) != tt.wantErr {
				t.Errorf("parseQueueStatus() error = %v, wantErr %v", delta.Err, tt.wantErr)
			}

			if delta.IsRetryable != tt.wantRetry {
				t.Errorf("parseQueueStatus() IsRetryable = %v, want %v", delta.IsRetryable, tt.wantRetry)
			}

			if delta.Err != nil && tt.errContains != "" {
				errMsg := delta.Err.Error()
				if !contains(errMsg, tt.errContains) {
					t.Errorf("parseQueueStatus() error = %v, should contain %v", errMsg, tt.errContains)
				}
			}
		})
	}
}

func TestCallQoderRetriesNested10605AfterServerWait(t *testing.T) {
	const serverWait = 100 * time.Millisecond

	var mu sync.Mutex
	requestCount := 0
	var firstRequestAt, secondRequestAt time.Time
	firstStreamCanceled := make(chan struct{}, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		attempt := requestCount
		if attempt == 1 {
			firstRequestAt = time.Now()
		} else {
			secondRequestAt = time.Now()
		}
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		if attempt == 1 {
			fmt.Fprintf(w, "data: %s\n\n", nested10605DataLine(t, serverWait))
			flusher.Flush()

			select {
			case <-r.Context().Done():
				firstStreamCanceled <- struct{}{}
			case <-time.After(time.Second):
				t.Error("first SSE stream was not canceled after nested 10605")
			}
			return
		}

		fmt.Fprintf(w, "data: %s\n\n", successfulDeltaDataLine(t, "OK"))
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	bridge := newTestBridge(t, server.URL)
	var content string
	if err := bridge.CallQoder(context.Background(), "claude", []interface{}{
		map[string]interface{}{"role": "user", "content": "test"},
	}, "dmodel", nil, func(delta Delta) {
		content += delta.Content
	}); err != nil {
		t.Fatalf("CallQoder() error = %v", err)
	}

	select {
	case <-firstStreamCanceled:
	case <-time.After(time.Second):
		t.Fatal("first SSE stream cancellation was not observed")
	}

	mu.Lock()
	gotRequestCount := requestCount
	elapsed := secondRequestAt.Sub(firstRequestAt)
	mu.Unlock()
	if gotRequestCount != 2 {
		t.Fatalf("request count = %d, want 2", gotRequestCount)
	}
	if elapsed < serverWait {
		t.Fatalf("retry waited %v, want at least server wait %v", elapsed, serverWait)
	}
	if elapsed > 800*time.Millisecond {
		t.Fatalf("retry waited too long (%v); first SSE should be canceled immediately", elapsed)
	}
	if content != "OK" {
		t.Fatalf("content = %q, want %q", content, "OK")
	}
}

func newTestBridge(t *testing.T, chatStreamURL string) *Bridge {
	t.Helper()
	sess := &cosy.SessionContext{
		CosyKey: "test-cosy-key",
		Info:    "test-session-info",
		Identity: cosy.AuthIdentity{
			Uid:      "test-user",
			UserType: "teams",
		},
		MachineId:    "test-machine-id",
		MachineToken: "test-machine-token",
		MachineType:  "test-machine-type",
	}
	return &Bridge{
		sess:   sess,
		client: &BearerClient{sess: sess, httpClient: &http.Client{}},
		templateBase: map[string]interface{}{
			"model_config": map[string]interface{}{"source": "system"},
			"parameters":   map[string]interface{}{},
			"business":     map[string]interface{}{},
			"chat_context": map[string]interface{}{"text": map[string]interface{}{}},
		},
		chatStreamURL: chatStreamURL,
	}
}

func nested10605DataLine(t *testing.T, wait time.Duration) string {
	t.Helper()
	queueInfo, err := json.Marshal(map[string]interface{}{
		"isQueued":         true,
		"modelKey":         "dmodel",
		"queueCount":       1,
		"queueType":        "slow",
		"serviceAvailable": true,
		"waitTime":         wait.Milliseconds(),
	})
	if err != nil {
		t.Fatal(err)
	}
	nested, err := json.Marshal(map[string]string{"code": "10605", "message": string(queueInfo)})
	if err != nil {
		t.Fatal(err)
	}
	outer, err := json.Marshal(map[string]string{"code": "403", "message": string(nested)})
	if err != nil {
		t.Fatal(err)
	}
	wrapper, err := json.Marshal(map[string]string{"body": string(outer)})
	if err != nil {
		t.Fatal(err)
	}
	return string(wrapper)
}

func successfulDeltaDataLine(t *testing.T, content string) string {
	t.Helper()
	inner, err := json.Marshal(map[string]interface{}{
		"choices": []interface{}{map[string]interface{}{
			"delta": map[string]interface{}{"content": content},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	wrapper, err := json.Marshal(map[string]string{"body": string(inner)})
	if err != nil {
		t.Fatal(err)
	}
	return string(wrapper)
}

func TestExtractDelta_NestedErrorCode10605(t *testing.T) {
	// 截图中的真实形态：外层 403，message 中嵌套业务码及队列状态。
	queueInfo, err := json.Marshal(map[string]interface{}{
		"isQueued":          false,
		"modelKey":          "dmodel",
		"queueCount":        0,
		"queueType":         "slow",
		"retryAfterSeconds": 0,
		"serviceAvailable":  true,
		"waitTime":          0,
	})
	if err != nil {
		t.Fatal(err)
	}
	nested, err := json.Marshal(map[string]string{"code": "10605", "message": string(queueInfo)})
	if err != nil {
		t.Fatal(err)
	}
	outer, err := json.Marshal(map[string]string{"code": "403", "message": string(nested)})
	if err != nil {
		t.Fatal(err)
	}
	dataLine, err := json.Marshal(map[string]string{"body": string(outer)})
	if err != nil {
		t.Fatal(err)
	}

	delta := ExtractDelta(string(dataLine))
	if delta.Err == nil {
		t.Fatal("expected nested 10605 to produce an error")
	}
	if !delta.IsRetryable {
		t.Fatalf("nested 10605 must be retryable, got %v", delta.Err)
	}
	if !contains(delta.Err.Error(), "temporarily throttled") {
		t.Fatalf("unexpected error: %v", delta.Err)
	}
	if !isRetryableError(delta.Err) {
		t.Fatalf("nested 10605 error must be structurally retryable: %v", delta.Err)
	}
}

func TestRetryDelay(t *testing.T) {
	if got := retryDelay(2_000_000_000, 1, 500_000_000); got != 500_000_000 {
		t.Fatalf("server retry-after should win: got %v", got)
	}
	if got := retryDelay(2_000_000_000, 3, 0); got != 8_000_000_000 {
		t.Fatalf("expected exponential fallback of 8s, got %v", got)
	}
}

func TestExtractDelta_ErrorCode10605(t *testing.T) {
	// 模拟真实的 Qoder API 错误响应
	queueInfo := map[string]interface{}{
		"isQueued":          false,
		"modelKey":          "dmodel",
		"queueCount":        0,
		"queueType":         "slow",
		"retryAfterSeconds": 0,
		"serviceAvailable":  true,
		"waitTime":          0,
	}
	queueJSON, _ := json.Marshal(queueInfo)

	innerJSON := map[string]interface{}{
		"code":    "10605",
		"message": string(queueJSON),
	}
	innerStr, _ := json.Marshal(innerJSON)

	wrapper := map[string]interface{}{
		"body": string(innerStr),
	}
	dataLine, _ := json.Marshal(wrapper)

	delta := ExtractDelta(string(dataLine))

	if delta.Err == nil {
		t.Error("Expected error for code 10605")
	}

	if !delta.IsRetryable {
		t.Error("Error code 10605 should be retryable")
	}

	if delta.Err != nil && !contains(delta.Err.Error(), "throttled") {
		t.Errorf("Error message should indicate throttling, got: %v", delta.Err)
	}
}

func TestExtractDelta_OtherRetryableErrors(t *testing.T) {
	tests := []struct {
		code      string
		wantRetry bool
	}{
		{"10605", true},
		{"429", true},
		{"503", true},
		{"500", false},
		{"115", false},
	}

	for _, tt := range tests {
		t.Run("code_"+tt.code, func(t *testing.T) {
			innerJSON := map[string]interface{}{
				"code":    tt.code,
				"message": "test error",
			}
			innerStr, _ := json.Marshal(innerJSON)

			wrapper := map[string]interface{}{
				"body": string(innerStr),
			}
			dataLine, _ := json.Marshal(wrapper)

			delta := ExtractDelta(string(dataLine))

			if delta.Err == nil {
				t.Errorf("Expected error for code %s", tt.code)
			}

			if delta.IsRetryable != tt.wantRetry {
				t.Errorf("Code %s: IsRetryable = %v, want %v", tt.code, delta.IsRetryable, tt.wantRetry)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
