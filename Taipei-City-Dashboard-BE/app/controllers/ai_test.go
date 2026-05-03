package controllers

import (
	"TaipeiCityDashboardBE/app/models"
	"TaipeiCityDashboardBE/app/services/ai"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tmc/langchaingo/llms"
)

func TestChatWithTWCCStreamingIncludesComponentsEvent(t *testing.T) {
	orig := chatWithTWCCFn
	defer func() { chatWithTWCCFn = orig }()

	chatWithTWCCFn = func(ctx context.Context, req ai.AIChatRequest, options ...llms.CallOption) (*ai.ChatResult, error) {
		callOpts := llms.CallOptions{}
		for _, opt := range options {
			opt(&callOpts)
		}

		if callOpts.StreamingFunc != nil {
			_ = callOpts.StreamingFunc(ctx, []byte("data: {\"generated_text\":\"測試回覆\"}\n\n"))
			_ = callOpts.StreamingFunc(ctx, []byte("data: [DONE]\n\n"))
		}

		return &ai.ChatResult{
			Log: &models.AIChatLog{
				SessionID: req.SessionID,
				Answer:    "測試回覆",
			},
			RecommendedComponents: []models.CityComponentScore{
				{ID: 1, Index: "population", Name: "人口圖表", City: "taipei", Score: 0.91},
			},
		}, nil
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/ai/chat/twai", ChatWithTWCC)

	body := `{
		"stream": true,
		"messages": [
			{"role":"user","content":"幫我找人口圖表"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/chat/twai", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	respBody := w.Body.String()
	if !strings.Contains(respBody, "event: components") {
		t.Fatalf("expected components event in stream, got: %s", respBody)
	}
	if !strings.Contains(respBody, "population") {
		t.Fatalf("expected components payload in stream, got: %s", respBody)
	}
}

func TestChatWithTWCCStreamingWithoutComponentsEvent(t *testing.T) {
	orig := chatWithTWCCFn
	defer func() { chatWithTWCCFn = orig }()

	chatWithTWCCFn = func(ctx context.Context, req ai.AIChatRequest, options ...llms.CallOption) (*ai.ChatResult, error) {
		callOpts := llms.CallOptions{}
		for _, opt := range options {
			opt(&callOpts)
		}

		if callOpts.StreamingFunc != nil {
			_ = callOpts.StreamingFunc(ctx, []byte("data: {\"generated_text\":\"純文字回覆\"}\n\n"))
			_ = callOpts.StreamingFunc(ctx, []byte("data: [DONE]\n\n"))
		}

		return &ai.ChatResult{
			Log: &models.AIChatLog{
				SessionID: req.SessionID,
				Answer:    "純文字回覆",
			},
			RecommendedComponents: []models.CityComponentScore{},
		}, nil
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/ai/chat/twai", ChatWithTWCC)

	body := `{
		"stream": true,
		"messages": [
			{"role":"user","content":"上週垃圾車是否異常"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/chat/twai", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	respBody := w.Body.String()
	if strings.Contains(respBody, "event: components") {
		t.Fatalf("did not expect components event in stream, got: %s", respBody)
	}
}

func TestChatWithTWCCStreamingIncludesToolCallsEvent(t *testing.T) {
	orig := chatWithTWCCFn
	defer func() { chatWithTWCCFn = orig }()

	chatWithTWCCFn = func(ctx context.Context, req ai.AIChatRequest, options ...llms.CallOption) (*ai.ChatResult, error) {
		callOpts := llms.CallOptions{}
		for _, opt := range options {
			opt(&callOpts)
		}

		if callOpts.StreamingFunc != nil {
			_ = callOpts.StreamingFunc(ctx, []byte("data: {\"generated_text\":\"查詢完成\"}\n\n"))
			_ = callOpts.StreamingFunc(ctx, []byte("data: [DONE]\n\n"))
		}

		return &ai.ChatResult{
			Log: &models.AIChatLog{
				SessionID: req.SessionID,
				Answer:    "查詢完成",
				ToolUsed:  true,
				Tools:     `["search_components","get_chart_data"]`,
			},
		}, nil
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/ai/chat/twai", ChatWithTWCC)

	body := `{
		"stream": true,
		"messages": [
			{"role":"user","content":"幫我分析垃圾收運趨勢"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/chat/twai", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	respBody := w.Body.String()
	if !strings.Contains(respBody, "event: tool_calls") {
		t.Fatalf("expected tool_calls event in stream, got: %s", respBody)
	}
	if !strings.Contains(respBody, "search_components") {
		t.Fatalf("expected tool call names in stream payload, got: %s", respBody)
	}
}

func TestChatWithTWCCNonStreamingIncludesToolCalls(t *testing.T) {
	orig := chatWithTWCCFn
	defer func() { chatWithTWCCFn = orig }()

	chatWithTWCCFn = func(ctx context.Context, req ai.AIChatRequest, options ...llms.CallOption) (*ai.ChatResult, error) {
		return &ai.ChatResult{
			Log: &models.AIChatLog{
				SessionID: "session_test",
				Answer:    "已完成查詢",
				ToolUsed:  true,
				Tools:     `["search_components"]`,
			},
		}, nil
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/ai/chat/twai", ChatWithTWCC)

	body := `{
		"stream": false,
		"messages": [
			{"role":"user","content":"請推薦人口圖表"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/chat/twai", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	data, ok := payload["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("response data not found: %v", payload)
	}
	toolCalls, ok := data["tool_calls"].([]interface{})
	if !ok || len(toolCalls) != 1 || toolCalls[0] != "search_components" {
		t.Fatalf("unexpected tool_calls value: %v", data["tool_calls"])
	}
}

func TestChatWithTWCCStreamingPassesThroughSSEDataLines(t *testing.T) {
	orig := chatWithTWCCFn
	defer func() { chatWithTWCCFn = orig }()

	chatWithTWCCFn = func(ctx context.Context, req ai.AIChatRequest, options ...llms.CallOption) (*ai.ChatResult, error) {
		callOpts := llms.CallOptions{}
		for _, opt := range options {
			opt(&callOpts)
		}

		if callOpts.StreamingFunc != nil {
			_ = callOpts.StreamingFunc(ctx, []byte("data:{\"generated_text\":\"我\",\"details\":null,\"finish_reason\":null}\n\n"))
		}

		return &ai.ChatResult{
			Log: &models.AIChatLog{
				SessionID: req.SessionID,
				Answer:    "我",
			},
		}, nil
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/ai/chat/twai", ChatWithTWCC)

	body := `{
		"stream": true,
		"messages": [
			{"role":"user","content":"測試串流"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/chat/twai", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	respBody := w.Body.String()
	if !strings.Contains(respBody, "data:{\"generated_text\":\"我\"") {
		t.Fatalf("expected raw SSE data line passthrough, got: %s", respBody)
	}
	if strings.Contains(respBody, "\"generated_text\":\"data:{\\\"generated_text\\\":\\\"我\\\"") {
		t.Fatalf("did not expect double-wrapped generated_text payload, got: %s", respBody)
	}
}

func TestChatWithTWCCStreamingPassesThroughSSEBlankLineSeparator(t *testing.T) {
	orig := chatWithTWCCFn
	defer func() { chatWithTWCCFn = orig }()

	chatWithTWCCFn = func(ctx context.Context, req ai.AIChatRequest, options ...llms.CallOption) (*ai.ChatResult, error) {
		callOpts := llms.CallOptions{}
		for _, opt := range options {
			opt(&callOpts)
		}

		if callOpts.StreamingFunc != nil {
			_ = callOpts.StreamingFunc(ctx, []byte("data:{\"generated_text\":\"根\",\"details\":null,\"finish_reason\":null}\n"))
			_ = callOpts.StreamingFunc(ctx, []byte("\n"))
			_ = callOpts.StreamingFunc(ctx, []byte("data:{\"generated_text\":\"據\",\"details\":null,\"finish_reason\":null}\n"))
			_ = callOpts.StreamingFunc(ctx, []byte("\n"))
		}

		return &ai.ChatResult{
			Log: &models.AIChatLog{
				SessionID: req.SessionID,
				Answer:    "根據",
			},
		}, nil
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/ai/chat/twai", ChatWithTWCC)

	body := `{
		"stream": true,
		"messages": [
			{"role":"user","content":"測試串流分隔"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/chat/twai", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	respBody := w.Body.String()
	if strings.Contains(respBody, "\"generated_text\":\"\\n\"") {
		t.Fatalf("did not expect blank-line separator wrapped as generated_text, got: %s", respBody)
	}
	if !strings.Contains(respBody, "data:{\"generated_text\":\"根\"") {
		t.Fatalf("expected first SSE data line, got: %s", respBody)
	}
	if !strings.Contains(respBody, "data:{\"generated_text\":\"據\"") {
		t.Fatalf("expected second SSE data line, got: %s", respBody)
	}
}
