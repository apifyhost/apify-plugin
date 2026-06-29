package main

import (
	"context"
	"encoding/json"

	sdk "github.com/apifyhost/apify-plugin/plugin"
)

type customHeaderPlugin struct{}

type config struct {
	HeaderName  string `json:"header_name"`
	HeaderValue string `json:"header_value"`
}

func main() {
	if err := sdk.Serve(customHeaderPlugin{}); err != nil {
		panic(err)
	}
}

func (customHeaderPlugin) Descriptor() sdk.Descriptor {
	return sdk.Descriptor{
		Name:         "custom-header",
		Version:      "1.0.0",
		Runtime:      sdk.RuntimeExternalProcess,
		SDKVersion:   ">=0.1.0 <0.2.0",
		Protocol:     "http-json-v1",
		Phases:       []sdk.Phase{sdk.PhaseResponse},
		ConfigSchema: json.RawMessage(`{"type":"object","additionalProperties":true}`),
		Capabilities: []string{"http_response"},
	}
}

func (customHeaderPlugin) ValidateConfig(ctx context.Context, raw json.RawMessage) error {
	var cfg config
	return json.Unmarshal(raw, &cfg)
}

func (customHeaderPlugin) Execute(ctx context.Context, phase sdk.Phase, pc *sdk.Context, raw json.RawMessage) (*sdk.Result, error) {
	var cfg config
	_ = json.Unmarshal(raw, &cfg)
	if cfg.HeaderName == "" {
		cfg.HeaderName = "X-Apify-Plugin"
	}
	if cfg.HeaderValue == "" {
		cfg.HeaderValue = "enabled"
	}
	return &sdk.Result{
		Decision: sdk.DecisionContinue,
		Headers:  map[string]string{cfg.HeaderName: cfg.HeaderValue},
	}, nil
}
