package tools

import (
	"TaipeiCityDashboardBE/app/models"
	"context"
	"fmt"
	"strings"

	"github.com/tmc/langchaingo/llms"
)

var getComponentByQueryVectorFn = models.GetComponentByQueryVector

type SearchComponentsArgs struct {
	Query          string  `json:"query"`
	Limit          int     `json:"limit"`
	ScoreThreshold float64 `json:"score_threshold"`
}

func SearchComponentsTool() llms.Tool {
	return llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "search_components",
			Description: "依使用者描述搜尋相似圖表組件，回傳可推薦的卡片候選。適用於「想看哪些圖表」類問題。",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "搜尋關鍵字或自然語句，例如：人口、垃圾收運、交通壅塞。",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "回傳上限，建議 1-10，最大 30。",
					},
					"score_threshold": map[string]interface{}{
						"type":        "number",
						"description": "相似度門檻，範圍 0 到 1，預設 0.8。",
					},
				},
				"required": []string{"query"},
			},
		},
	}
}

func SearchComponents(ctx context.Context, args string) (string, error) {
	params := SearchComponentsArgs{
		Limit:          10,
		ScoreThreshold: 0.8,
	}
	if strings.TrimSpace(args) != "" {
		if err := parseArgs(args, &params); err != nil {
			return "", fmt.Errorf("invalid arguments: %v", err)
		}
	}

	params.Query = strings.TrimSpace(params.Query)
	if params.Query == "" {
		return "", fmt.Errorf("query is required")
	}
	if params.Limit <= 0 {
		params.Limit = 10
	}
	if params.Limit > 30 {
		params.Limit = 30
	}
	if params.ScoreThreshold <= 0 || params.ScoreThreshold > 1 {
		params.ScoreThreshold = 0.8
	}

	components, err := getComponentByQueryVectorFn(params.Query, params.Limit, params.ScoreThreshold)
	if err != nil {
		return "", fmt.Errorf("search components failed: %w", err)
	}

	normalized := normalizeRecommendedComponents(components)
	if runtime, ok := RuntimeFromContext(ctx); ok {
		runtime.SetRecommendedComponents(normalized)
	}

	if len(normalized) == 0 {
		return "找不到符合條件的圖表組件。請改用更具體關鍵字，或改成一般文字摘要回覆。", nil
	}

	lines := make([]string, 0, len(normalized)+1)
	lines = append(lines, fmt.Sprintf("找到 %d 個推薦組件：", len(normalized)))
	for i, component := range normalized {
		lines = append(lines, fmt.Sprintf(
			"%d. %s（id=%d, index=%s, city=%s, score=%.4f）",
			i+1,
			component.Name,
			component.ID,
			component.Index,
			component.City,
			component.Score,
		))
	}

	return strings.Join(lines, "\n"), nil
}
