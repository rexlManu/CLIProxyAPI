package cliproxy

import (
	internalconfig "github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"testing"
)

func TestEarliestResetDefaultRouting(t *testing.T) {
	state := normalizedRoutingRuntimeState(&internalconfig.Config{})
	if _, ok := newRoutingSelector(state).(*coreauth.EarliestResetSelector); !ok {
		t.Fatalf("default selector = %T", newRoutingSelector(state))
	}
	state = normalizedRoutingRuntimeState(&internalconfig.Config{Routing: internalconfig.RoutingConfig{Strategy: "round-robin"}})
	if _, ok := newRoutingSelector(state).(*coreauth.RoundRobinSelector); !ok {
		t.Fatalf("round-robin selector = %T", newRoutingSelector(state))
	}
}
