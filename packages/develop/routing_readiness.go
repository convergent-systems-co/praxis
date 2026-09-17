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
	return g.Routes.RecordReadiness(ctx, request)
}
