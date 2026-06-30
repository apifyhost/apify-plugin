package main

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestValidateManifestReadsReferencedSchemas(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "apify-plugin.yaml")
	writeFile(t, manifestPath, `apiVersion: apify.apifyhost.com/v1alpha1
kind: Plugin
metadata:
  name: custom-header
  version: 1.0.0
spec:
  runtime: external_process
  protocol: http-json-v1
  phases:
    - response
  configSchema: ./config.schema.json
  secretSchema: ./secret.schema.json
`)
	writeFile(t, filepath.Join(dir, "config.schema.json"), `{"type":"object"}`)
	writeFile(t, filepath.Join(dir, "secret.schema.json"), `{"type":"object","additionalProperties":false}`)

	m, err := readManifest(manifestPath)
	if err != nil {
		t.Fatalf("readManifest() error = %v", err)
	}
	if err := validateManifest(m, dir); err != nil {
		t.Fatalf("validateManifest() error = %v", err)
	}
}

func TestValidateManifestRejectsInvalidSchemaJSON(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "apify-plugin.yaml")
	writeFile(t, manifestPath, `metadata:
  name: custom-header
  version: 1.0.0
spec:
  runtime: external_process
  protocol: http-json-v1
  phases:
    - response
  configSchema: ./config.schema.json
`)
	writeFile(t, filepath.Join(dir, "config.schema.json"), `{`)

	m, err := readManifest(manifestPath)
	if err != nil {
		t.Fatalf("readManifest() error = %v", err)
	}
	err = validateManifest(m, dir)
	if err == nil || !strings.Contains(err.Error(), "config.schema.json is not valid JSON") {
		t.Fatalf("validateManifest() error = %v, want invalid JSON error", err)
	}
}

func TestPackDirectoryWritesRelativeArchiveEntries(t *testing.T) {
	sourceDir := t.TempDir()
	outputDir := t.TempDir()
	writeFile(t, filepath.Join(sourceDir, "apify-plugin.yaml"), "metadata:\n  name: custom-header\n")
	writeFile(t, filepath.Join(sourceDir, "main.go"), "package main\n")
	writeFile(t, filepath.Join(sourceDir, "schemas", "config.schema.json"), `{"type":"object"}`)

	output := filepath.Join(outputDir, "custom-header.tar.gz")
	if err := packDirectory(sourceDir, output); err != nil {
		t.Fatalf("packDirectory() error = %v", err)
	}

	got := archiveEntryNames(t, output)
	want := []string{"apify-plugin.yaml", "main.go", "schemas/config.schema.json"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("archive entries = %v, want %v", got, want)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func archiveEntryNames(t *testing.T, path string) []string {
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

	var names []string
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read tar entry: %v", err)
		}
		names = append(names, header.Name)
	}
	sort.Strings(names)
	return names
}
