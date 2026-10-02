package config

import "testing"

func TestQuotaRoutingAndWebsocketConfigLayouts(t *testing.T) {
	for _, data := range []string{
		"routing: {strategy: earliest-reset}\ncodex: {enable-websocket-upstream: true}\n",
		"routing: {strategy: earliest-reset}\noauth: {providers: {codex: {enable-websocket-upstream: true}}}\n",
	} {
		cfg, errParse := ParseConfigBytes([]byte(data))
		if errParse != nil {
			t.Fatal(errParse)
		}
		if cfg.Routing.Strategy != "earliest-reset" || !cfg.Codex.EnableWebsocketUpstream {
			t.Fatalf("settings did not survive parsing: routing=%+v, codex=%+v", cfg.Routing, cfg.Codex)
		}
	}
	cfg, errParse := ParseConfigBytes([]byte("routing: {strategy: round-robin}\n"))
	if errParse != nil {
		t.Fatal(errParse)
	}
	if cfg.Routing.Strategy != "round-robin" || cfg.Codex.EnableWebsocketUpstream {
		t.Fatal("explicit routing or default websocket setting changed")
	}
}
