package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type stubPlugin struct {
	validateErr error
	result      *Result
	executeErr  error
}

func (p stubPlugin) Descriptor() Descriptor {
	return Descriptor{
		Name:       "stub",
		Version:    "1.0.0",
		Runtime:    RuntimeExternalProcess,
		Protocol:   "http-json-v1",
		Phases:     []Phase{PhaseResponse},
		SDKVersion: ">=0.1.0 <0.2.0",
	}
}

func (p stubPlugin) ValidateConfig(context.Context, json.RawMessage) error {
	return p.validateErr
}

func (p stubPlugin) Execute(context.Context, Phase, *Context, json.RawMessage) (*Result, error) {
	return p.result, p.executeErr
}

func TestHandlerServesPluginEndpoints(t *testing.T) {
	handler, err := newHandler(stubPlugin{
		result: &Result{
			Decision: DecisionContinue,
			Headers:  map[string]string{"X-Test": "ok"},
		},
	})
	if err != nil {
		t.Fatalf("newHandler() error = %v", err)
	}

	resp := performRequest(handler, http.MethodGet, "/health", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /health status = %d, want %d", resp.Code, http.StatusOK)
	}

	resp = performRequest(handler, http.MethodGet, "/describe", nil)
	var descriptor Descriptor
	if err := json.NewDecoder(resp.Body).Decode(&descriptor); err != nil {
		t.Fatalf("decode descriptor: %v", err)
	}
	if descriptor.Name != "stub" || descriptor.Version != "1.0.0" {
		t.Fatalf("descriptor = %#v", descriptor)
	}

	validateBody := []byte(`{"config":{"enabled":true}}`)
	resp = performRequest(handler, http.MethodPost, "/validate-config", validateBody)
	var validateResp ValidateConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&validateResp); err != nil {
		t.Fatalf("decode validate response: %v", err)
	}
	if !validateResp.Valid {
		t.Fatalf("validate response valid = false, want true: %#v", validateResp)
	}

	executeReq := ExecuteRequest{
		RequestID:     "req-1",
		Phase:         PhaseResponse,
		ExecutionKind: ExecutionKindHTTPRequest,
		Context:       Context{Kind: ExecutionKindHTTPRequest},
		Config:        json.RawMessage(`{"enabled":true}`),
	}
	rawExecuteReq, err := json.Marshal(executeReq)
	if err != nil {
		t.Fatalf("marshal execute request: %v", err)
	}
	resp = performRequest(handler, http.MethodPost, "/execute", rawExecuteReq)
	var executeResp ExecuteResponse
	if err := json.NewDecoder(resp.Body).Decode(&executeResp); err != nil {
		t.Fatalf("decode execute response: %v", err)
	}
	if executeResp.Result == nil || executeResp.Result.Headers["X-Test"] != "ok" {
		t.Fatalf("execute response = %#v", executeResp)
	}
}

func TestHandlerReportsPluginErrors(t *testing.T) {
	handler, err := newHandler(stubPlugin{
		validateErr: errors.New("bad config"),
		executeErr:  errors.New("boom"),
	})
	if err != nil {
		t.Fatalf("newHandler() error = %v", err)
	}

	resp := performRequest(handler, http.MethodPost, "/validate-config", []byte(`{"config":{}}`))
	var validateResp ValidateConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&validateResp); err != nil {
		t.Fatalf("decode validate response: %v", err)
	}
	if validateResp.Valid || validateResp.ErrorCode != "invalid_plugin_config" {
		t.Fatalf("validate response = %#v", validateResp)
	}

	executeReq := []byte(`{"phase":"response","execution_kind":"http_request","context":{"kind":"http_request"},"config":{}}`)
	resp = performRequest(handler, http.MethodPost, "/execute", executeReq)
	var executeResp ExecuteResponse
	if err := json.NewDecoder(resp.Body).Decode(&executeResp); err != nil {
		t.Fatalf("decode execute response: %v", err)
	}
	if executeResp.Result == nil || executeResp.Result.Decision != DecisionAbort || executeResp.Result.ErrorCode != "plugin_execution_failed" {
		t.Fatalf("execute response = %#v", executeResp)
	}
}

func TestNewHandlerRejectsInvalidPlugin(t *testing.T) {
	if _, err := newHandler(nil); err == nil {
		t.Fatal("newHandler(nil) error = nil, want error")
	}
}

func performRequest(handler http.Handler, method string, path string, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
