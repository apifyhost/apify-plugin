//go:build e2e

package e2e

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/apifyhost/apify-plugin/plugin"
)

func TestCustomHeaderPluginEndToEnd(t *testing.T) {
	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	exampleDir := filepath.Join(repoRoot, "examples", "plugins", "custom-header")
	tmpDir := t.TempDir()

	runGo(t, repoRoot, "run", "./cmd/apify-plugin", "validate", "--manifest", filepath.Join(exampleDir, "apify-plugin.yaml"))

	binaryPath := filepath.Join(tmpDir, "custom-header")
	runGo(t, repoRoot, "run", "./cmd/apify-plugin", "build", "--dir", exampleDir, "--output", binaryPath)

	archivePath := filepath.Join(tmpDir, "custom-header.tar.gz")
	runGo(t, repoRoot, "run", "./cmd/apify-plugin", "pack", "--dir", exampleDir, "--output", archivePath)
	assertArchiveContains(t, archivePath, []string{
		"README.md",
		"apify-plugin.yaml",
		"config.schema.json",
		"main.go",
		"secret.schema.json",
	})

	addr := freeTCPAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binaryPath)
	cmd.Env = append(os.Environ(), "APIFY_PLUGIN_ADDR="+addr)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start plugin binary: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
		if t.Failed() {
			t.Logf("plugin stdout:\n%s", stdout.String())
			t.Logf("plugin stderr:\n%s", stderr.String())
		}
	})

	baseURL := "http://" + addr
	client := &http.Client{Timeout: 2 * time.Second}
	waitForHealth(t, client, baseURL+"/health")
	assertDescriptor(t, client, baseURL+"/describe")
	assertValidConfig(t, client, baseURL+"/validate-config")
	assertExecuteAddsHeader(t, client, baseURL+"/execute")
}

func runGo(t *testing.T, dir string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		t.Fatalf("go %v failed: %v\n%s", args, err, output.String())
	}
}

func assertArchiveContains(t *testing.T, path string, want []string) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("open gzip reader: %v", err)
	}
	defer gzipReader.Close()

	got := make(map[string]struct{})
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read tar entry: %v", err)
		}
		got[header.Name] = struct{}{}
	}
	for _, name := range want {
		if _, ok := got[name]; !ok {
			t.Fatalf("archive is missing %s; entries = %v", name, got)
		}
	}
}

func freeTCPAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if os.Getenv("CI") == "" && strings.Contains(err.Error(), "operation not permitted") {
			t.Skipf("skipping runtime HTTP check because localhost listen is not permitted: %v", err)
			return ""
		}
		t.Fatalf("listen on free TCP port: %v", err)
		return ""
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	return addr
}

func waitForHealth(t *testing.T, client *http.Client, url string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("plugin did not become healthy at %s", url)
}

func assertDescriptor(t *testing.T, client *http.Client, url string) {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET /describe: %v", err)
	}
	defer resp.Body.Close()

	var descriptor sdk.Descriptor
	if err := json.NewDecoder(resp.Body).Decode(&descriptor); err != nil {
		t.Fatalf("decode descriptor: %v", err)
	}
	if descriptor.Name != "custom-header" || descriptor.Version != "1.0.0" {
		t.Fatalf("descriptor = %#v", descriptor)
	}
}

func assertValidConfig(t *testing.T, client *http.Client, url string) {
	t.Helper()
	request := sdk.ValidateConfigRequest{
		OrganizationID: "org-1",
		PluginName:     "custom-header",
		PluginVersion:  "1.0.0",
		Config:         json.RawMessage(`{"header_name":"X-E2E","header_value":"ok"}`),
	}
	var response sdk.ValidateConfigResponse
	postJSON(t, client, url, request, &response)
	if !response.Valid {
		t.Fatalf("validate response = %#v", response)
	}
}

func assertExecuteAddsHeader(t *testing.T, client *http.Client, url string) {
	t.Helper()
	request := sdk.ExecuteRequest{
		RequestID:     "req-e2e",
		Phase:         sdk.PhaseResponse,
		ExecutionKind: sdk.ExecutionKindHTTPRequest,
		Context: sdk.Context{
			Kind: sdk.ExecutionKindHTTPRequest,
			HTTP: &sdk.HTTPContext{
				Method: "GET",
				Path:   "/widgets",
			},
		},
		Config: json.RawMessage(`{"header_name":"X-E2E","header_value":"ok"}`),
	}
	var response sdk.ExecuteResponse
	postJSON(t, client, url, request, &response)
	if response.Result == nil || response.Result.Decision != sdk.DecisionContinue || response.Result.Headers["X-E2E"] != "ok" {
		t.Fatalf("execute response = %#v", response)
	}
}

func postJSON(t *testing.T, client *http.Client, url string, request any, response any) {
	t.Helper()
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	resp, err := client.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST %s status = %d, body = %s", url, resp.StatusCode, body)
	}
	if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}
