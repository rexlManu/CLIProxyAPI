package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestClaudeAgentSDKCloakingConfigRoundTrip(t *testing.T) {
	raw := []byte("config-version: 8\nupstream:\n  claude:\n    cloak-agent-sdk: true\n")
	if errValidate := ValidateV8Config(raw); errValidate != nil {
		t.Fatal(errValidate)
	}
	cfg, errParse := ParseConfigBytes(raw)
	if errParse != nil {
		t.Fatal(errParse)
	}
	serialized, errMarshal := yaml.Marshal(cfg)
	if errMarshal != nil {
		t.Fatal(errMarshal)
	}
	restored, errParse := ParseConfigBytes(serialized)
	if errParse != nil {
		t.Fatal(errParse)
	}
	for _, view := range []*Config{cfg, cfg.ForAPIKey(), cfg.CloneForRuntime(), restored} {
		if !view.Claude.CloakAgentSDK {
			t.Fatal("SDK cloak policy was lost during config conversion")
		}
	}
}
