package develop

import (
	"context"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/routingauthority"
)

func TestWeatherRouteReadinessRequiresIssuedRouteLedger(t *testing.T) {
	if _, err := (WeatherRouteReadinessGate{}).Qualify(context.Background(), routingauthority.ReadinessRequest{}); err == nil {
		t.Fatal("weather readiness accepted caller input without the core issued-route ledger")
	}
}
