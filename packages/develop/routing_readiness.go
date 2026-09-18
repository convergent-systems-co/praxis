package develop

import (
	"context"
	"errors"

	"github.com/convergent-systems-co/praxis/internal/inference"
	"github.com/convergent-systems-co/praxis/internal/routingauthority"
)

// WeatherRouteReadinessGate records the governed route required by the
// weather-dashboard workflow. It deliberately has no execution surface.
type WeatherRouteReadinessGate struct {
	Routes *routingauthority.IssuedRouteLedger
}

func (g WeatherRouteReadinessGate) Qualify(ctx context.Context, request routingauthority.ReadinessRequest) (inference.UnifiedRouteRecord, error) {
	if g.Routes == nil {
		return inference.UnifiedRouteRecord{}, errors.New("weather route readiness requires an issued-route ledger")
	}
	graph := Graph()
	if request.Request.GraphID != graph.ID || request.Request.GraphVersion != graph.Version || request.Request.NodeID != "implement" || request.Request.GoalRef == "" {
		return inference.UnifiedRouteRecord{}, errors.New("weather route readiness requires exact Develop implement-node and Goal lineage")
	}
	return g.Routes.RecordReadiness(ctx, request)
}
