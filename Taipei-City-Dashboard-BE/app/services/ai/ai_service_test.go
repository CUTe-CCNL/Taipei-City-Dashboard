package ai

import (
	"TaipeiCityDashboardBE/app/models"
	"TaipeiCityDashboardBE/app/services/ai/tools"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"
)

type toolCallModelStub struct {
	toolName      string
	callCount     int
	sawToolResult bool
}

type captureToolsModelStub struct {
	callCount      int
	firstCallTools int
}

func (m *toolCallModelStub) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	m.callCount++

	if m.callCount == 1 {
		return &llms.ContentResponse{
			Choices: []*llms.ContentChoice{{
				Content: "",
				GenerationInfo: map[string]interface{}{
					"tool_calls": []llms.ToolCall{
						{
							ID:   "call_test_1",
							Type: "function",
							FunctionCall: &llms.FunctionCall{
								Name:      m.toolName,
								Arguments: `{"query":"人口"}`,
							},
						},
					},
					"usage": map[string]interface{}{
						"input_tokens":  10,
						"output_tokens": 2,
						"total_tokens":  12,
					},
				},
			}},
		}, nil
	}

	m.sawToolResult = hasToolResponse(messages, "call_test_1", "TOOL_RESULT")

	return &llms.ContentResponse{
		Choices: []*llms.ContentChoice{{
			Content: "這是整合工具結果後的最終回答",
			GenerationInfo: map[string]interface{}{
				"tool_calls": []llms.ToolCall{},
				"usage": map[string]interface{}{
					"input_tokens":  20,
					"output_tokens": 8,
					"total_tokens":  28,
				},
			},
		}},
	}, nil
}

func (m *toolCallModelStub) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	return "", nil
}

func (m *captureToolsModelStub) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	m.callCount++
	callOpts := llms.CallOptions{}
	for _, opt := range options {
		opt(&callOpts)
	}
	if m.callCount == 1 {
		m.firstCallTools = len(callOpts.Tools)
	}

	return &llms.ContentResponse{
		Choices: []*llms.ContentChoice{{
			Content: "OK",
			GenerationInfo: map[string]interface{}{
				"tool_calls": []llms.ToolCall{},
				"usage": map[string]interface{}{
					"input_tokens":  1,
					"output_tokens": 1,
					"total_tokens":  2,
				},
			},
		}},
	}, nil
}

func (m *captureToolsModelStub) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	return "", nil
}

func hasToolResponse(messages []llms.MessageContent, toolCallID string, expectedContent string) bool {
	for _, msg := range messages {
		if msg.Role != llms.ChatMessageTypeTool {
			continue
		}
		for _, part := range msg.Parts {
			resp, ok := part.(llms.ToolCallResponse)
			if !ok {
				continue
			}
			if resp.ToolCallID == toolCallID && strings.Contains(resp.Content, expectedContent) {
				return true
			}
		}
	}
	return false
}

func TestChatWithTWCCExecutesToolCalls(t *testing.T) {
	origModel := twccModel
	origCreateLog := createAIChatLogFn
	defer func() {
		twccModel = origModel
		createAIChatLogFn = origCreateLog
	}()

	createAIChatLogFn = func(log *models.AIChatLog) error { return nil }

	toolName := "test_tool_call_execution"
	var called int
	var gotArgs string
	tools.Register(toolName, func(ctx context.Context, args string) (string, error) {
		called++
		gotArgs = args
		return "TOOL_RESULT", nil
	})

	model := &toolCallModelStub{toolName: toolName}
	twccModel = model

	req := AIChatRequest{
		SessionID: "session_test_toolcall",
		UserID:    "1",
		IPAddress: "127.0.0.1",
		Messages: []llms.MessageContent{
			{
				Role: llms.ChatMessageTypeHuman,
				Parts: []llms.ContentPart{
					llms.TextContent{Text: "請幫我查人口相關圖表"},
				},
			},
		},
	}

	options := []llms.CallOption{
		llms.WithTools([]llms.Tool{
			{
				Type: "function",
				Function: &llms.FunctionDefinition{
					Name:        toolName,
					Description: "test tool",
					Parameters: map[string]interface{}{
						"type":       "object",
						"properties": map[string]interface{}{"query": map[string]interface{}{"type": "string"}},
					},
				},
			},
		}),
	}

	result, err := ChatWithTWCC(context.Background(), req, options...)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result == nil || result.Log == nil {
		t.Fatalf("expected non-nil chat result")
	}

	if called != 1 {
		t.Fatalf("expected tool to be executed once, got %d", called)
	}
	if gotArgs != `{"query":"人口"}` {
		t.Fatalf("unexpected tool args: %s", gotArgs)
	}
	if model.callCount != 2 {
		t.Fatalf("expected model to be called twice (before and after tool), got %d", model.callCount)
	}
	if !model.sawToolResult {
		t.Fatalf("expected second model call to include tool response message")
	}
	if !result.Log.ToolUsed {
		t.Fatalf("expected tool_used=true")
	}

	var usedTools []string
	if err := json.Unmarshal([]byte(result.Log.Tools), &usedTools); err != nil {
		t.Fatalf("failed to parse tools json: %v", err)
	}
	if len(usedTools) != 1 || usedTools[0] != toolName {
		t.Fatalf("unexpected used tools: %v", usedTools)
	}
}

func TestChatWithTWCCInjectsDefaultToolsWhenMissing(t *testing.T) {
	origModel := twccModel
	origCreateLog := createAIChatLogFn
	defer func() {
		twccModel = origModel
		createAIChatLogFn = origCreateLog
	}()

	createAIChatLogFn = func(log *models.AIChatLog) error { return nil }

	model := &captureToolsModelStub{}
	twccModel = model

	req := AIChatRequest{
		SessionID: "session_test_default_tools",
		UserID:    "1",
		IPAddress: "127.0.0.1",
		Messages: []llms.MessageContent{
			{
				Role: llms.ChatMessageTypeHuman,
				Parts: []llms.ContentPart{
					llms.TextContent{Text: "台北空氣品質最差的地區是哪裡？"},
				},
			},
		},
	}

	result, err := ChatWithTWCC(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result == nil || result.Log == nil {
		t.Fatalf("expected non-nil chat result")
	}
	if model.callCount == 0 {
		t.Fatalf("model was not called")
	}
	if model.firstCallTools == 0 {
		t.Fatalf("expected default tools to be injected when call options miss tools")
	}
}
