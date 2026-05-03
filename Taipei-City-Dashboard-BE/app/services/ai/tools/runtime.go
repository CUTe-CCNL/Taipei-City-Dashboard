package tools

import (
	"TaipeiCityDashboardBE/app/models"
	"context"
	"sort"
	"sync"
)

type runtimeContextKey struct{}

// Runtime stores tool execution metadata that the controller can use after model generation.
type Runtime struct {
	mu                    sync.RWMutex
	recommendedComponents []models.CityComponentScore
}

func NewRuntime() *Runtime {
	return &Runtime{}
}

func WithRuntime(ctx context.Context, runtime *Runtime) context.Context {
	if runtime == nil {
		return ctx
	}
	return context.WithValue(ctx, runtimeContextKey{}, runtime)
}

func RuntimeFromContext(ctx context.Context) (*Runtime, bool) {
	runtime, ok := ctx.Value(runtimeContextKey{}).(*Runtime)
	return runtime, ok && runtime != nil
}

func (r *Runtime) SetRecommendedComponents(components []models.CityComponentScore) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recommendedComponents = normalizeRecommendedComponents(components)
}

func (r *Runtime) RecommendedComponents() []models.CityComponentScore {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]models.CityComponentScore, len(r.recommendedComponents))
	copy(out, r.recommendedComponents)
	return out
}

func normalizeRecommendedComponents(components []models.CityComponentScore) []models.CityComponentScore {
	deduped := make(map[string]models.CityComponentScore)
	for _, component := range components {
		existing, exists := deduped[component.Index]
		if !exists {
			deduped[component.Index] = component
			continue
		}

		// Keep metrotaipei if duplicate index exists (front-end historical behavior).
		if component.City == "metrotaipei" && existing.City != "metrotaipei" {
			deduped[component.Index] = component
			continue
		}

		if component.Score > existing.Score {
			deduped[component.Index] = component
		}
	}

	out := make([]models.CityComponentScore, 0, len(deduped))
	for _, component := range deduped {
		out = append(out, component)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})
	return out
}
