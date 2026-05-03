package tools

import (
	"TaipeiCityDashboardBE/app/models"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tmc/langchaingo/llms"
)

var (
	getComponentByIDFn           = models.GetComponentByID
	getComponentChartDataQueryFn = models.GetComponentChartDataQuery
	getTwoDimensionalDataFn      = models.GetTwoDimensionalData
	getThreeDimensionalDataFn    = models.GetThreeDimensionalData
	getTimeSeriesDataFn          = models.GetTimeSeriesData
	getMapLegendDataFn           = models.GetMapLegendData
)

type GetChartDataArgs struct {
	ComponentID int    `json:"component_id"`
	TimeRange   string `json:"time_range"`
	City        string `json:"city"`
}

func GetChartDataTool() llms.Tool {
	return llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "get_chart_data",
			Description: "查詢指定圖表組件的資料摘要。time_range 目前支援 last_24h, today, last_week, last_month。",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"component_id": map[string]interface{}{
						"type":        "integer",
						"description": "組件 ID。",
					},
					"time_range": map[string]interface{}{
						"type":        "string",
						"description": "時間範圍，僅可為 last_24h, today, last_week, last_month。",
						"enum":        []string{"last_24h", "today", "last_week", "last_month"},
					},
					"city": map[string]interface{}{
						"type":        "string",
						"description": "城市代碼，可為 taipei 或 metrotaipei，預設 taipei。",
					},
				},
				"required": []string{"component_id", "time_range"},
			},
		},
	}
}

func GetChartData(ctx context.Context, args string) (string, error) {
	_ = ctx

	var params GetChartDataArgs
	if err := parseArgs(args, &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %v", err)
	}

	if params.ComponentID <= 0 {
		return "", fmt.Errorf("component_id must be a positive integer")
	}

	if params.City == "" {
		params.City = "taipei"
	}
	if params.City != "taipei" && params.City != "metrotaipei" {
		return "", fmt.Errorf("city must be taipei or metrotaipei")
	}

	timeFrom, timeTo, err := parseTimeRange(params.TimeRange)
	if err != nil {
		return "", err
	}

	component, err := getComponentByIDFn(params.ComponentID, params.City)
	if err != nil {
		return "", fmt.Errorf("找不到組件 id=%d（city=%s）: %w", params.ComponentID, params.City, err)
	}

	queryType, queryString, err := getComponentChartDataQueryFn(params.ComponentID, params.City)
	if err != nil {
		return "", fmt.Errorf("查詢組件資料失敗: %w", err)
	}
	if strings.TrimSpace(queryType) == "" || strings.TrimSpace(queryString) == "" {
		return fmt.Sprintf("組件「%s」目前沒有可查詢的圖表資料。", component.Name), nil
	}

	timeWindow := fmt.Sprintf("%s ~ %s", timeFrom.Format("2006-01-02 15:04"), timeTo.Format("2006-01-02 15:04"))
	timeFromStr := timeFrom.Format("2006-01-02T15:04:05+08:00")
	timeToStr := timeTo.Format("2006-01-02T15:04:05+08:00")

	switch queryType {
	case "two_d":
		data, err := getTwoDimensionalDataFn(&queryString, timeFromStr, timeToStr)
		if err != nil {
			return "", fmt.Errorf("查詢組件資料失敗: %w", err)
		}
		if len(data) == 0 || len(data[0].Data) == 0 {
			return fmt.Sprintf("組件「%s」在 %s 無資料。", component.Name, timeWindow), nil
		}
		summary := summarizeTwoDimensional(data[0].Data)
		return fmt.Sprintf("【%s】\n時間範圍：%s\n%s", component.Name, timeWindow, summary), nil

	case "three_d", "percent":
		data, categories, err := getThreeDimensionalDataFn(&queryString, timeFromStr, timeToStr)
		if err != nil {
			return "", fmt.Errorf("查詢組件資料失敗: %w", err)
		}
		if len(data) == 0 {
			return fmt.Sprintf("組件「%s」在 %s 無資料。", component.Name, timeWindow), nil
		}
		summary := summarizeThreeDimensional(data, categories)
		return fmt.Sprintf("【%s】\n時間範圍：%s\n%s", component.Name, timeWindow, summary), nil

	case "time":
		data, err := getTimeSeriesDataFn(&queryString, timeFromStr, timeToStr)
		if err != nil {
			return "", fmt.Errorf("查詢組件資料失敗: %w", err)
		}
		if len(data) == 0 {
			return fmt.Sprintf("組件「%s」在 %s 無資料。", component.Name, timeWindow), nil
		}
		summary := summarizeTimeSeries(data)
		return fmt.Sprintf("【%s】\n時間範圍：%s\n%s", component.Name, timeWindow, summary), nil

	case "map_legend":
		data, err := getMapLegendDataFn(&queryString, timeFromStr, timeToStr)
		if err != nil {
			return "", fmt.Errorf("查詢組件資料失敗: %w", err)
		}
		if len(data) == 0 {
			return fmt.Sprintf("組件「%s」在 %s 無資料。", component.Name, timeWindow), nil
		}
		summary := summarizeMapLegend(data)
		return fmt.Sprintf("【%s】\n時間範圍：%s\n%s", component.Name, timeWindow, summary), nil
	}

	return fmt.Sprintf("組件「%s」的資料型別 %s 目前尚未支援摘要。", component.Name, queryType), nil
}

func parseTimeRange(timeRange string) (time.Time, time.Time, error) {
	loc, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		loc = time.FixedZone("UTC+8", 8*60*60)
	}
	now := time.Now().In(loc)

	switch timeRange {
	case "last_24h":
		return now.Add(-24 * time.Hour), now, nil
	case "today":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		return start, now, nil
	case "last_week":
		return now.AddDate(0, 0, -7), now, nil
	case "last_month":
		return now.AddDate(0, -1, 0), now, nil
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("time_range must be one of: last_24h, today, last_week, last_month")
	}
}

func summarizeTwoDimensional(points []models.TwoDimensionalData) string {
	min := points[0]
	max := points[0]
	for _, point := range points[1:] {
		if point.Data < min.Data {
			min = point
		}
		if point.Data > max.Data {
			max = point
		}
	}

	return fmt.Sprintf(
		"資料筆數：%d\n最大值：%s = %.2f\n最小值：%s = %.2f",
		len(points),
		max.Xaxis,
		max.Data,
		min.Xaxis,
		min.Data,
	)
}

func summarizeThreeDimensional(series []models.ThreeDimensionalDataOutput, categories []string) string {
	topName := ""
	topValue := -1
	for _, item := range series {
		for _, val := range item.Data {
			if val > topValue {
				topValue = val
				topName = item.Name
			}
		}
	}

	return fmt.Sprintf(
		"類別數：%d\n系列數：%d\n最高值系列：%s (%d)",
		len(categories),
		len(series),
		topName,
		topValue,
	)
}

func summarizeTimeSeries(series []models.TimeSeriesDataOutput) string {
	totalPoints := 0
	lines := make([]string, 0, len(series)+1)
	for _, item := range series {
		totalPoints += len(item.Data)
		if len(item.Data) == 0 {
			continue
		}
		last := item.Data[len(item.Data)-1]
		lines = append(lines, fmt.Sprintf("- %s：最新值 %.2f（%s）", item.Name, last.Y, last.X))
	}
	if len(lines) > 4 {
		lines = lines[:4]
	}
	return fmt.Sprintf("系列數：%d，資料點總數：%d\n%s", len(series), totalPoints, strings.Join(lines, "\n"))
}

func summarizeMapLegend(items []models.MapLegendData) string {
	max := items[0]
	for _, item := range items[1:] {
		if item.Value > max.Value {
			max = item
		}
	}

	return fmt.Sprintf(
		"圖例項目數：%d\n最高值：%s = %.2f（%s）",
		len(items),
		max.Name,
		max.Value,
		max.Type,
	)
}
