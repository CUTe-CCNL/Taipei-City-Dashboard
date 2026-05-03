package ai

import (
	"TaipeiCityDashboardBE/app/models"
	"TaipeiCityDashboardBE/app/services/ai/providers/twcc"
	"TaipeiCityDashboardBE/app/services/ai/tools"
	"TaipeiCityDashboardBE/global"
	"TaipeiCityDashboardBE/logs"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tmc/langchaingo/llms"
	"golang.org/x/sync/semaphore"
)

var (
	// aiSemaphore limits the number of concurrent AI requests
	aiSemaphore       *semaphore.Weighted
	twccModel         llms.Model
	createAIChatLogFn = models.CreateAIChatLog
)

func init() {
	aiSemaphore = semaphore.NewWeighted(int64(global.TWCC.MaxConcurrent))
	twccModel = twcc.New(
		global.TWCC.ApiKey,
		global.TWCC.ApiUrl,
		global.TWCC.Model,
		global.TWCC.Timeout,
	)
}

type AIChatRequest struct {
	SessionID string                 `json:"session"`
	UserID    string                 `json:"user_id"`
	IPAddress string                 `json:"ip_address"`
	Messages  []llms.MessageContent  `json:"messages"`
	Params    map[string]interface{} `json:"params"`
}

type ChatResult struct {
	Log                   *models.AIChatLog
	RecommendedComponents []models.CityComponentScore
}

// ChatWithTWCC handles the AI conversation logic including retries, tool calling loop, and logging.
func ChatWithTWCC(ctx context.Context, req AIChatRequest, options ...llms.CallOption) (*ChatResult, error) {
	if err := aiSemaphore.Acquire(ctx, 1); err != nil {
		return nil, fmt.Errorf("server too busy: %v", err)
	}
	defer aiSemaphore.Release(1)

	session := newSession(req, options...)
	return session.run(ctx)
}

func newSession(req AIChatRequest, options ...llms.CallOption) *aiSession {
	s := &aiSession{
		req:             req,
		options:         options,
		currentMessages: make([]llms.MessageContent, 0),
		startTime:       time.Now(),
		runtime:         tools.NewRuntime(),
	}
	for _, opt := range options {
		opt(&s.callOpts)
	}
	s.ensureToolOptions()
	s.injectInstructions()
	return s
}

func (s *aiSession) ensureToolOptions() {
	if len(s.callOpts.Tools) > 0 {
		return
	}

	defaultTools := tools.DefaultTools()
	if len(defaultTools) == 0 {
		return
	}

	s.callOpts.Tools = defaultTools
	s.options = append(s.options, llms.WithTools(defaultTools))
	if s.callOpts.ToolChoice == nil {
		s.callOpts.ToolChoice = "auto"
		s.options = append(s.options, llms.WithToolChoice("auto"))
	}
}

type aiSession struct {
	req                   AIChatRequest
	options               []llms.CallOption
	callOpts              llms.CallOptions
	currentMessages       []llms.MessageContent
	totalInput            int
	totalOutput           int
	toolUsed              bool
	executedTools         []string
	lastResp              *llms.ContentResponse
	lastErr               error
	startTime             time.Time
	runtime               *tools.Runtime
	recommendedComponents []models.CityComponentScore
}

func (s *aiSession) run(ctx context.Context) (*ChatResult, error) {
	ctx = tools.WithRuntime(ctx, s.runtime)
	maxLoops := 5
	s.executedTools = make([]string, 0)
	for i := 0; i < maxLoops; i++ {
		s.sendHeartbeat(ctx)

		if err := s.generate(ctx); err != nil {
			break
		}

		toolCalls := s.extractToolCalls()
		if len(toolCalls) == 0 {
			break
		}

		s.toolUsed = true
		logs.FInfo("Loop %d: Processing %d tool calls", i, len(toolCalls))
		if err := s.executeTools(ctx, toolCalls); err != nil {
			break
		}
		s.syncRecommendedComponents()
	}
	s.syncRecommendedComponents()
	return s.finalize()
}

func (s *aiSession) sendHeartbeat(ctx context.Context) {
	if s.callOpts.StreamingFunc != nil {
		s.callOpts.StreamingFunc(ctx, []byte(": heartbeat\n\n"))
	}
}

func (s *aiSession) generate(ctx context.Context) error {
	maxRetry := global.TWCC.MaxRetry
	if s.callOpts.StreamingFunc != nil {
		maxRetry = 0
	}

	for i := 0; i <= maxRetry; i++ {
		s.lastResp, s.lastErr = twccModel.GenerateContent(ctx, s.currentMessages, s.options...)
		if s.lastErr == nil {
			s.updateTokens()
			return nil
		}
		logs.FError("Attempt %d failed: %v", i+1, s.lastErr)
		if i < maxRetry {
			time.Sleep(500 * time.Millisecond)
		}
	}
	return s.lastErr
}

func (s *aiSession) extractToolCalls() []llms.ToolCall {
	if s.lastResp == nil || len(s.lastResp.Choices) == 0 {
		return nil
	}
	tc, _ := s.lastResp.Choices[0].GenerationInfo["tool_calls"].([]llms.ToolCall)
	return tc
}

func (s *aiSession) updateTokens() {
	if s.lastResp == nil || len(s.lastResp.Choices) == 0 {
		return
	}
	if usage, ok := s.lastResp.Choices[0].GenerationInfo["usage"].(map[string]interface{}); ok {
		s.totalInput += parseUsageInt(usage["input_tokens"])
		s.totalOutput += parseUsageInt(usage["output_tokens"])
	}
}

func (s *aiSession) executeTools(ctx context.Context, toolCalls []llms.ToolCall) error {
	choice := s.lastResp.Choices[0]

	// Add Assistant's intent
	s.currentMessages = append(s.currentMessages, llms.MessageContent{
		Role:  llms.ChatMessageTypeAI,
		Parts: append([]llms.ContentPart{llms.TextContent{Text: choice.Content}}, toolsToParts(toolCalls)...),
	})

	for _, tc := range toolCalls {
		s.executedTools = append(s.executedTools, tc.FunctionCall.Name)
		result, err := tools.Execute(ctx, tc.FunctionCall.Name, tc.FunctionCall.Arguments)
		if err != nil {
			result = fmt.Sprintf("Error: %v. Please verify arguments.", err)
			logs.FError("Tool Error: %v", err)
		}

		s.currentMessages = append(s.currentMessages, llms.MessageContent{
			Role: llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{llms.ToolCallResponse{
				ToolCallID: tc.ID, Name: tc.FunctionCall.Name, Content: result,
			}},
		})
	}
	return nil
}

const civicAssistantSystemPrompt = `你是「臺北城市儀表板市政小助理」，必須使用繁體中文回覆，且以市政資料查詢為核心。
你的任務：
1. 若使用者想找圖表、比較主題或建立儀表板，優先呼叫 search_components。
2. 若使用者詢問特定指標趨勢、異常或區間數據，使用 get_chart_data（必要時先用 search_components 找到 component_id）。
3. 若使用者不確定可查哪些主題，可呼叫 get_dashboard 說明目前可用儀表板與組件。
4. 工具結果不足時要誠實說明資料限制，不要編造。
5. 請以清楚、精簡、可執行的語氣回覆，必要時提出下一步建議。`

func (s *aiSession) injectInstructions() {
	toolNames := ""
	for i, t := range s.callOpts.Tools {
		if i > 0 {
			toolNames += ", "
		}
		toolNames += t.Function.Name
	}
	if toolNames == "" {
		toolNames = "none"
	}

	instruction := fmt.Sprintf(
		"%s\n\nSystem Instruction:\n1. Use ONLY: [%s].\n2. NEVER nest tool calls.\n3. Arguments MUST be literal values (strings, integers, etc.), never function calls.\n4. For dependent tasks, call tools sequentially in separate turns.\n5. If stuck, respond with text.",
		civicAssistantSystemPrompt,
		toolNames,
	)

	s.currentMessages = make([]llms.MessageContent, 0)
	merged := false
	for _, m := range s.req.Messages {
		if m.Role == llms.ChatMessageTypeSystem && !merged {
			s.currentMessages = append(s.currentMessages, mergeSystemMsg(m, instruction))
			merged = true
		} else {
			s.currentMessages = append(s.currentMessages, m)
		}
	}

	if !merged {
		s.currentMessages = append([]llms.MessageContent{{
			Role:  llms.ChatMessageTypeSystem,
			Parts: []llms.ContentPart{llms.TextContent{Text: instruction}},
		}}, s.currentMessages...)
	}
}

func (s *aiSession) syncRecommendedComponents() {
	if s.runtime == nil {
		return
	}
	s.recommendedComponents = s.runtime.RecommendedComponents()
}

func (s *aiSession) finalize() (*ChatResult, error) {
	log := &models.AIChatLog{
		SessionID: s.req.SessionID, UserID: s.req.UserID, IPAddress: s.req.IPAddress,
		Provider: "twcc", Model: global.TWCC.Model, LatencyMS: int(time.Since(s.startTime).Milliseconds()),
		Status: "success", Tools: "[]", CreatedAt: s.startTime,
	}

	if len(s.req.Messages) > 0 {
		log.Question = extractText(s.req.Messages[len(s.req.Messages)-1])
	}

	if s.lastErr != nil {
		log.Status, log.ErrorCode, log.ErrorMessage = "error", "MODEL_ERROR", s.lastErr.Error()
		createAIChatLogFn(log)
		return &ChatResult{
			Log:                   log,
			RecommendedComponents: s.recommendedComponents,
		}, s.lastErr
	}

	if s.lastResp != nil && len(s.lastResp.Choices) > 0 {
		log.Answer = s.lastResp.Choices[0].Content
		log.InputTokens, log.OutputTokens = s.totalInput, s.totalOutput
		log.TotalTokens = s.totalInput + s.totalOutput
		if s.toolUsed {
			log.ToolUsed = true
			if toolJSON, err := json.Marshal(s.executedTools); err == nil {
				log.Tools = string(toolJSON)
			}
		}
	}

	if err := createAIChatLogFn(log); err != nil {
		logs.FError("DB Log Error: %v", err)
	}
	return &ChatResult{
		Log:                   log,
		RecommendedComponents: s.recommendedComponents,
	}, nil
}

func toolsToParts(calls []llms.ToolCall) []llms.ContentPart {
	parts := make([]llms.ContentPart, len(calls))
	for i, c := range calls {
		parts[i] = c
	}
	return parts
}

func mergeSystemMsg(m llms.MessageContent, instruction string) llms.MessageContent {
	newParts := make([]llms.ContentPart, 0, len(m.Parts)+1)
	merged := false
	for i, p := range m.Parts {
		_ = i
		if tp, ok := p.(llms.TextContent); ok {
			newParts = append(newParts, llms.TextContent{Text: tp.Text + "\n\n" + instruction})
			merged = true
		} else {
			newParts = append(newParts, p)
		}
	}
	if !merged {
		newParts = append(newParts, llms.TextContent{Text: instruction})
	}
	return llms.MessageContent{Role: m.Role, Parts: newParts}
}

func extractText(m llms.MessageContent) string {
	for _, p := range m.Parts {
		if t, ok := p.(llms.TextContent); ok {
			return t.Text
		}
	}
	return ""
}

func parseUsageInt(val interface{}) int {
	switch v := val.(type) {
	case int:
		return v
	case float64:
		return int(v)
	default:
		return 0
	}
}
