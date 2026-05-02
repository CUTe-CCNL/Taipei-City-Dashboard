package tools

import (
	"TaipeiCityDashboardBE/app/models"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSearchComponentsInvalidArgs(t *testing.T) {
	_, err := SearchComponents(context.Background(), "{")
	if err == nil {
		t.Fatalf("expected parse error, got nil")
	}
}

func TestSearchComponentsSuccessAndRuntime(t *testing.T) {
	orig := getComponentByQueryVectorFn
	defer func() { getComponentByQueryVectorFn = orig }()

	getComponentByQueryVectorFn = func(queryString string, limit int, scoreThreshold float64) ([]models.CityComponentScore, error) {
		return []models.CityComponentScore{
			{ID: 1, Index: "population", Name: "人口趨勢", City: "taipei", Score: 0.89},
			{ID: 2, Index: "population", Name: "人口趨勢（雙北）", City: "metrotaipei", Score: 0.86},
			{ID: 3, Index: "garbage", Name: "垃圾收運量", City: "taipei", Score: 0.82},
		}, nil
	}

	runtime := NewRuntime()
	ctx := WithRuntime(context.Background(), runtime)
	out, err := SearchComponents(ctx, `{"query":"人口","limit":5}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "人口趨勢") {
		t.Fatalf("expected output to contain component name, got: %s", out)
	}

	components := runtime.RecommendedComponents()
	if len(components) != 2 {
		t.Fatalf("expected deduped components length=2, got=%d", len(components))
	}
	if components[0].City != "metrotaipei" {
		t.Fatalf("expected metrotaipei overwrite on duplicate index")
	}
}

func TestGetChartDataNotFoundComponent(t *testing.T) {
	origGetComponentByID := getComponentByIDFn
	defer func() { getComponentByIDFn = origGetComponentByID }()

	getComponentByIDFn = func(id int, city string) (models.CityComponent, error) {
		return models.CityComponent{}, errors.New("record not found")
	}

	_, err := GetChartData(context.Background(), `{"component_id":999,"time_range":"last_week"}`)
	if err == nil {
		t.Fatalf("expected not found error, got nil")
	}
}

func TestGetChartDataNoData(t *testing.T) {
	origGetComponentByID := getComponentByIDFn
	origGetComponentChartDataQuery := getComponentChartDataQueryFn
	origGetTimeSeriesData := getTimeSeriesDataFn
	defer func() {
		getComponentByIDFn = origGetComponentByID
		getComponentChartDataQueryFn = origGetComponentChartDataQuery
		getTimeSeriesDataFn = origGetTimeSeriesData
	}()

	getComponentByIDFn = func(id int, city string) (models.CityComponent, error) {
		return models.CityComponent{Name: "垃圾收運量"}, nil
	}
	getComponentChartDataQueryFn = func(id int, city string) (string, string, error) {
		return "time", "SELECT ...", nil
	}
	getTimeSeriesDataFn = func(query *string, timeFrom string, timeTo string) ([]models.TimeSeriesDataOutput, error) {
		return []models.TimeSeriesDataOutput{}, nil
	}

	out, err := GetChartData(context.Background(), `{"component_id":1,"time_range":"last_week"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "無資料") {
		t.Fatalf("expected no-data message, got: %s", out)
	}
}

func TestGetChartDataSuccess(t *testing.T) {
	origGetComponentByID := getComponentByIDFn
	origGetComponentChartDataQuery := getComponentChartDataQueryFn
	origGetTimeSeriesData := getTimeSeriesDataFn
	defer func() {
		getComponentByIDFn = origGetComponentByID
		getComponentChartDataQueryFn = origGetComponentChartDataQuery
		getTimeSeriesDataFn = origGetTimeSeriesData
	}()

	getComponentByIDFn = func(id int, city string) (models.CityComponent, error) {
		return models.CityComponent{Name: "垃圾收運量"}, nil
	}
	getComponentChartDataQueryFn = func(id int, city string) (string, string, error) {
		return "time", "SELECT ...", nil
	}
	getTimeSeriesDataFn = func(query *string, timeFrom string, timeTo string) ([]models.TimeSeriesDataOutput, error) {
		return []models.TimeSeriesDataOutput{
			{
				Name: "收運量",
				Data: []models.TimeSeriesDataItem{
					{X: "2026-05-01T00:00:00+08:00", Y: 123},
					{X: "2026-05-02T00:00:00+08:00", Y: 140},
				},
			},
		}, nil
	}

	out, err := GetChartData(context.Background(), `{"component_id":1,"time_range":"last_week"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "系列數") {
		t.Fatalf("expected summary output, got: %s", out)
	}
}

func TestGetDashboardInvalidArgs(t *testing.T) {
	_, err := GetDashboard(context.Background(), "{")
	if err == nil {
		t.Fatalf("expected parse error, got nil")
	}
}

func TestGetDashboardSuccess(t *testing.T) {
	orig := getDashboardCatalogFn
	defer func() { getDashboardCatalogFn = orig }()

	getDashboardCatalogFn = func() ([]dashboardGroup, error) {
		return []dashboardGroup{
			{
				Name: "public",
				Dashboards: []models.Dashboard{
					{Name: "交通儀表板", Index: "traffic", Components: []int64{1, 2, 3}},
				},
			},
		}, nil
	}

	out, err := GetDashboard(context.Background(), `{"scope":"public","limit":3}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "交通儀表板") {
		t.Fatalf("expected dashboard entry in output, got: %s", out)
	}
}
