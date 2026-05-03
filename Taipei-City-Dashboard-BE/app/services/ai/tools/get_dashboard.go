package tools

import (
	"TaipeiCityDashboardBE/app/models"
	"context"
	"fmt"
	"strings"

	"github.com/tmc/langchaingo/llms"
)

type GetDashboardArgs struct {
	Scope string `json:"scope"`
	Limit int    `json:"limit"`
}

type dashboardGroup struct {
	Name       string
	Dashboards []models.Dashboard
}

var getDashboardCatalogFn = defaultGetDashboardCatalog

func GetDashboardTool() llms.Tool {
	return llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "get_dashboard",
			Description: "列出目前可用儀表板與其組件清單，協助判斷可查詢主題與組件範圍。",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"scope": map[string]interface{}{
						"type":        "string",
						"description": "可選 all/public/taipei/metrotaipei。",
						"enum":        []string{"all", "public", "taipei", "metrotaipei"},
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "每個群組最多列出的儀表板數量，預設 10，最大 30。",
					},
				},
			},
		},
	}
}

func GetDashboard(ctx context.Context, args string) (string, error) {
	_ = ctx

	params := GetDashboardArgs{
		Scope: "all",
		Limit: 10,
	}
	if strings.TrimSpace(args) != "" {
		if err := parseArgs(args, &params); err != nil {
			return "", fmt.Errorf("invalid arguments: %v", err)
		}
	}

	if params.Scope == "" {
		params.Scope = "all"
	}
	if params.Limit <= 0 {
		params.Limit = 10
	}
	if params.Limit > 30 {
		params.Limit = 30
	}

	groups, err := getDashboardCatalogFn()
	if err != nil {
		return "", fmt.Errorf("查詢儀表板清單失敗: %w", err)
	}

	filtered := filterDashboardGroups(groups, params.Scope)
	lines := make([]string, 0, 8)
	for _, group := range filtered {
		if len(group.Dashboards) == 0 {
			continue
		}

		lines = append(lines, fmt.Sprintf("【%s】共 %d 個儀表板", group.Name, len(group.Dashboards)))
		limit := params.Limit
		if len(group.Dashboards) < limit {
			limit = len(group.Dashboards)
		}
		for i := 0; i < limit; i++ {
			dashboard := group.Dashboards[i]
			lines = append(lines, fmt.Sprintf(
				"- %s（index=%s，components=%d，ids=%s）",
				dashboard.Name,
				dashboard.Index,
				len(dashboard.Components),
				formatComponentIDs(dashboard.Components),
			))
		}
	}

	if len(lines) == 0 {
		return "目前沒有可用的儀表板資料。", nil
	}

	return strings.Join(lines, "\n"), nil
}

func defaultGetDashboardCatalog() ([]dashboardGroup, error) {
	dashboards, err := models.GetAllDashboards(0)
	if err != nil {
		return nil, err
	}
	return []dashboardGroup{
		{Name: "public", Dashboards: dashboards.Public},
		{Name: "taipei", Dashboards: dashboards.Taipei},
		{Name: "metrotaipei", Dashboards: dashboards.MetroTaipei},
	}, nil
}

func filterDashboardGroups(groups []dashboardGroup, scope string) []dashboardGroup {
	if scope == "all" {
		return groups
	}

	filtered := make([]dashboardGroup, 0, len(groups))
	for _, group := range groups {
		if group.Name == scope {
			filtered = append(filtered, group)
		}
	}
	return filtered
}

func formatComponentIDs(ids []int64) string {
	if len(ids) == 0 {
		return "-"
	}
	limit := len(ids)
	if limit > 8 {
		limit = 8
	}

	parts := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		parts = append(parts, fmt.Sprintf("%d", ids[i]))
	}
	if len(ids) > limit {
		parts = append(parts, "...")
	}

	return strings.Join(parts, ",")
}
