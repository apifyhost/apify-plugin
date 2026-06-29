package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"
)

type ExecutionKind string

const (
	ExecutionKindHTTPRequest ExecutionKind = "http_request"
	ExecutionKindWorkflow    ExecutionKind = "workflow"
)

type Phase string

const (
	PhaseRoute        Phase = "route"
	PhaseBeforeAuth   Phase = "before_auth"
	PhaseAfterAuth    Phase = "after_auth"
	PhaseBeforeDataOP Phase = "before_data_op"
	PhaseAfterDataOP  Phase = "after_data_op"
	PhaseResponse     Phase = "response"
	PhaseLog          Phase = "log"

	PhaseWorkflowTrigger Phase = "workflow_trigger"
	PhaseWorkflowPrepare Phase = "workflow_prepare"
	PhaseWorkflowExecute Phase = "workflow_execute"
	PhaseWorkflowSuccess Phase = "workflow_success"
	PhaseWorkflowFailure Phase = "workflow_failure"
	PhaseWorkflowFinally Phase = "workflow_finally"
)

type Runtime string

const (
	RuntimeBuiltin         Runtime = "builtin"
	RuntimeExternalProcess Runtime = "external_process"
	RuntimeWebhook         Runtime = "webhook"
	RuntimeWASM            Runtime = "wasm"
)

type Descriptor struct {
	Name         string          `json:"name"`
	Version      string          `json:"version"`
	Runtime      Runtime         `json:"runtime"`
	SDKVersion   string          `json:"sdk_version"`
	Protocol     string          `json:"protocol"`
	Phases       []Phase         `json:"phases"`
	ConfigSchema json.RawMessage `json:"config_schema"`
	SecretSchema json.RawMessage `json:"secret_schema"`
	Capabilities []string        `json:"capabilities"`
}

type Decision string

const (
	DecisionContinue   Decision = "continue"
	DecisionAbort      Decision = "abort"
	DecisionSkipDataOP Decision = "skip_data_op"
	DecisionRetry      Decision = "retry"
)

type Result struct {
	Decision      Decision          `json:"decision"`
	StatusCode    int               `json:"status_code,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	Body          []byte            `json:"body,omitempty"`
	ErrorCode     string            `json:"error_code,omitempty"`
	ErrorMessage  string            `json:"error_msg,omitempty"`
	ErrorParams   map[string]any    `json:"error_params,omitempty"`
	AsyncLogAttrs map[string]any    `json:"async_log_attrs,omitempty"`
}

type Context struct {
	Kind           ExecutionKind    `json:"kind"`
	OrganizationID string           `json:"organization_id"`
	ServiceID      string           `json:"service_id"`
	APIID          string           `json:"api_id"`
	RequestID      string           `json:"request_id"`
	Plugin         *PluginContext   `json:"plugin,omitempty"`
	Principal      *Principal       `json:"principal,omitempty"`
	HTTP           *HTTPContext     `json:"http,omitempty"`
	DataOP         *DataOperation   `json:"data_op,omitempty"`
	Workflow       *WorkflowContext `json:"workflow,omitempty"`
	Vars           map[string]any   `json:"vars,omitempty"`
	Host           *HostServices    `json:"-"`
}

type PluginContext struct {
	Name          string    `json:"name"`
	Version       string    `json:"version"`
	BindingID     string    `json:"binding_id,omitempty"`
	ConfigID      string    `json:"config_id,omitempty"`
	ScopeType     string    `json:"scope_type,omitempty"`
	ScopeID       string    `json:"scope_id,omitempty"`
	OverrideKey   string    `json:"override_key,omitempty"`
	Priority      int       `json:"priority,omitempty"`
	FailurePolicy string    `json:"failure_policy,omitempty"`
	CreatedBy     string    `json:"created_by,omitempty"`
	CreatedAt     time.Time `json:"created_at,omitempty"`
	UpdatedAt     time.Time `json:"updated_at,omitempty"`
}

type Principal struct {
	ID    string `json:"id"`
	Email string `json:"email,omitempty"`
	Role  string `json:"role,omitempty"`
}

type HTTPContext struct {
	Method      string              `json:"method"`
	Path        string              `json:"path"`
	MatchedPath string              `json:"matched_path"`
	PathParams  map[string]string   `json:"path_params,omitempty"`
	Query       map[string][]string `json:"query,omitempty"`
	Headers     map[string]string   `json:"headers,omitempty"`
	ClientIP    string              `json:"client_ip,omitempty"`
}

type DataOperation struct {
	Method          string            `json:"method"`
	Resource        string            `json:"resource,omitempty"`
	Table           string            `json:"table,omitempty"`
	ResourceID      string            `json:"resource_id,omitempty"`
	RequestBody     json.RawMessage   `json:"request_body,omitempty"`
	ResponseBody    json.RawMessage   `json:"response_body,omitempty"`
	ResponseHeaders map[string]string `json:"response_headers,omitempty"`
	StatusCode      int               `json:"status_code,omitempty"`
}

type WorkflowContext struct {
	JobID          string         `json:"job_id"`
	TriggerPayload map[string]any `json:"trigger_payload,omitempty"`
	NodeOutputs    map[string]any `json:"node_outputs,omitempty"`
}

type HostServices struct {
	Cache CacheBackend
	State StateStore
}

type CacheBackend interface {
	Get(ctx context.Context, key string) (CacheEntry, bool)
	Set(ctx context.Context, key string, entry CacheEntry, ttl time.Duration, indexKeys []string) error
	Invalidate(ctx context.Context, indexKeys []string) int
}

type CacheEntry struct {
	Body      []byte
	Headers   map[string]string
	ExpiresAt time.Time
}

type StateStore interface {
	Get(key string) (any, bool)
	Set(key string, value any)
	Delete(key string)
}

type Plugin interface {
	Descriptor() Descriptor
	ValidateConfig(ctx context.Context, config json.RawMessage) error
	Execute(ctx context.Context, phase Phase, pc *Context, config json.RawMessage) (*Result, error)
}

type Starter interface {
	Start(ctx context.Context, config json.RawMessage) error
}

type Stopper interface {
	Stop(ctx context.Context) error
}

type ValidateConfigRequest struct {
	OrganizationID string            `json:"organization_id"`
	PluginName     string            `json:"plugin_name"`
	PluginVersion  string            `json:"plugin_version"`
	Config         json.RawMessage   `json:"config"`
	SecretRefs     map[string]string `json:"secret_refs,omitempty"`
}

type ValidateConfigResponse struct {
	Valid       bool           `json:"valid"`
	ErrorCode   string         `json:"error_code,omitempty"`
	ErrorMsg    string         `json:"error_msg,omitempty"`
	ErrorParams map[string]any `json:"error_params,omitempty"`
}

type ExecuteRequest struct {
	RequestID     string            `json:"request_id"`
	Phase         Phase             `json:"phase"`
	ExecutionKind ExecutionKind     `json:"execution_kind"`
	Context       Context           `json:"context"`
	Config        json.RawMessage   `json:"config"`
	SecretValues  map[string]string `json:"secret_values,omitempty"`
}

type ExecuteResponse struct {
	Result *Result `json:"result"`
}

func Serve(p Plugin) error {
	if p == nil {
		return errors.New("plugin is nil")
	}
	descriptor := p.Descriptor()
	if descriptor.Name == "" || descriptor.Version == "" {
		return errors.New("plugin descriptor requires name and version")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/describe", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(descriptor)
	})
	mux.HandleFunc("/validate-config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req ValidateConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ValidateConfigResponse{Valid: false, ErrorCode: "invalid_request_body", ErrorMsg: "invalid request body"})
			return
		}
		if err := p.ValidateConfig(r.Context(), req.Config); err != nil {
			writeJSON(w, http.StatusOK, ValidateConfigResponse{Valid: false, ErrorCode: "invalid_plugin_config", ErrorMsg: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, ValidateConfigResponse{Valid: true})
	})
	mux.HandleFunc("/execute", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req ExecuteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ExecuteResponse{Result: &Result{Decision: DecisionAbort, ErrorCode: "invalid_request_body", ErrorMessage: "invalid request body"}})
			return
		}
		result, err := p.Execute(r.Context(), req.Phase, &req.Context, req.Config)
		if err != nil {
			writeJSON(w, http.StatusOK, ExecuteResponse{Result: &Result{Decision: DecisionAbort, ErrorCode: "plugin_execution_failed", ErrorMessage: err.Error()}})
			return
		}
		if result == nil {
			result = &Result{Decision: DecisionContinue}
		}
		writeJSON(w, http.StatusOK, ExecuteResponse{Result: result})
	})
	addr := os.Getenv("APIFY_PLUGIN_ADDR")
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return server.ListenAndServe()
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
