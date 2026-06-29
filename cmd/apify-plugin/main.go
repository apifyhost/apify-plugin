package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type manifest struct {
	APIVersion string `yaml:"apiVersion" json:"apiVersion"`
	Kind       string `yaml:"kind" json:"kind"`
	Metadata   struct {
		Name        string `yaml:"name" json:"name"`
		Version     string `yaml:"version" json:"version"`
		DisplayName string `yaml:"displayName" json:"displayName"`
		Description string `yaml:"description" json:"description"`
	} `yaml:"metadata" json:"metadata"`
	Spec struct {
		Runtime      string   `yaml:"runtime" json:"runtime"`
		SDKVersion   string   `yaml:"sdkVersion" json:"sdkVersion"`
		Protocol     string   `yaml:"protocol" json:"protocol"`
		Entrypoint   string   `yaml:"entrypoint" json:"entrypoint"`
		Phases       []string `yaml:"phases" json:"phases"`
		Capabilities []string `yaml:"capabilities" json:"capabilities"`
		ConfigSchema string   `yaml:"configSchema" json:"configSchema"`
		SecretSchema string   `yaml:"secretSchema" json:"secretSchema"`
	} `yaml:"spec" json:"spec"`
}

func main() {
	root := &cobra.Command{
		Use:   "apify-plugin",
		Short: "Apify plugin development helper",
	}
	root.AddCommand(initCommand(), validateCommand(), buildCommand(), packCommand(), placeholderCommand("test", "run plugin harness tests"))
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func initCommand() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "init NAME",
		Short: "scaffold a plugin project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.TrimSpace(args[0])
			if name == "" {
				return errors.New("plugin name is required")
			}
			target := dir
			if target == "" {
				target = name
			}
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			files := map[string]string{
				"apify-plugin.yaml":  manifestTemplate(name),
				"config.schema.json": configSchemaTemplate(),
				"secret.schema.json": `{"type":"object","additionalProperties":false}` + "\n",
				"go.mod":             pluginGoModTemplate(name),
				"main.go":            pluginMainTemplate(name),
				"README.md":          "# " + name + "\n",
			}
			for path, content := range files {
				if err := os.WriteFile(filepath.Join(target, path), []byte(content), 0o644); err != nil {
					return err
				}
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "created plugin scaffold at %s\n", target)
			return err
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "target directory")
	return cmd
}

func validateCommand() *cobra.Command {
	var manifestPath string
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "validate plugin manifest and schemas",
		RunE: func(cmd *cobra.Command, args []string) error {
			if manifestPath == "" {
				manifestPath = "apify-plugin.yaml"
			}
			m, err := readManifest(manifestPath)
			if err != nil {
				return err
			}
			if err := validateManifest(m, filepath.Dir(manifestPath)); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "plugin manifest is valid")
			return err
		},
	}
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "manifest path")
	return cmd
}

func buildCommand() *cobra.Command {
	var dir string
	var output string
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "build",
		Short: "build an external-process plugin binary",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dir == "" {
				dir = "."
			}
			manifestPath := filepath.Join(dir, "apify-plugin.yaml")
			m, err := readManifest(manifestPath)
			if err != nil {
				return err
			}
			if err := validateManifest(m, dir); err != nil {
				return err
			}
			if output == "" {
				outputName := filepath.Base(strings.TrimPrefix(m.Spec.Entrypoint, "./"))
				if outputName == "." || outputName == string(filepath.Separator) {
					outputName = m.Metadata.Name
				}
				output = filepath.Join(dir, outputName)
			}
			if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			build := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", output, ".")
			build.Dir = dir
			build.Stdout = cmd.OutOrStdout()
			build.Stderr = cmd.ErrOrStderr()
			if err := build.Run(); err != nil {
				if ctx.Err() == context.DeadlineExceeded {
					return errors.New("plugin build timed out")
				}
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "built %s\n", output)
			return err
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "plugin directory")
	cmd.Flags().StringVarP(&output, "output", "o", "", "output binary path")
	cmd.Flags().DurationVar(&timeout, "timeout", 2*time.Minute, "build timeout")
	return cmd
}

func packCommand() *cobra.Command {
	var dir string
	var output string
	cmd := &cobra.Command{
		Use:   "pack",
		Short: "pack a plugin source archive",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dir == "" {
				dir = "."
			}
			manifestPath := filepath.Join(dir, "apify-plugin.yaml")
			m, err := readManifest(manifestPath)
			if err != nil {
				return err
			}
			if err := validateManifest(m, dir); err != nil {
				return err
			}
			if output == "" {
				output = m.Metadata.Name + "-" + m.Metadata.Version + ".tar.gz"
			}
			if err := packDirectory(dir, output); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "packed %s\n", output)
			return err
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "plugin directory")
	cmd.Flags().StringVarP(&output, "output", "o", "", "output tar.gz path")
	return cmd
}

func placeholderCommand(name, description string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: description,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("%s is not implemented yet", name)
		},
	}
}

func readManifest(path string) (manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return manifest{}, err
	}
	var m manifest
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		err = json.Unmarshal(raw, &m)
	default:
		err = yaml.Unmarshal(raw, &m)
	}
	if err != nil {
		return manifest{}, err
	}
	return m, nil
}

func validateManifest(m manifest, baseDir string) error {
	if strings.TrimSpace(m.Metadata.Name) == "" || strings.TrimSpace(m.Metadata.Version) == "" {
		return errors.New("metadata.name and metadata.version are required")
	}
	if m.Spec.Runtime == "" {
		return errors.New("spec.runtime is required")
	}
	if m.Spec.Protocol == "" {
		return errors.New("spec.protocol is required")
	}
	if len(m.Spec.Phases) == 0 {
		return errors.New("spec.phases is required")
	}
	for _, schemaPath := range []string{m.Spec.ConfigSchema, m.Spec.SecretSchema} {
		if schemaPath == "" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(baseDir, schemaPath))
		if err != nil {
			return err
		}
		if !json.Valid(raw) {
			return fmt.Errorf("%s is not valid JSON", schemaPath)
		}
	}
	return nil
}

func packDirectory(dir, output string) error {
	file, err := os.Create(output)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()
	gzipWriter := gzip.NewWriter(file)
	defer func() {
		_ = gzipWriter.Close()
	}()
	tarWriter := tar.NewWriter(gzipWriter)
	defer func() {
		_ = tarWriter.Close()
	}()
	return filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == filepath.Base(output) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() {
			_ = src.Close()
		}()
		_, err = io.Copy(tarWriter, src)
		return err
	})
}

func manifestTemplate(name string) string {
	return fmt.Sprintf(`apiVersion: apify.apifyhost.com/v1alpha1
kind: Plugin
metadata:
  name: %s
  version: 1.0.0
  displayName: %s
  description: ""
spec:
  runtime: external_process
  sdkVersion: ">=0.1.0 <0.2.0"
  protocol: http-json-v1
  entrypoint: ./%s
  phases:
    - response
  capabilities:
    - http_response
  configSchema: ./config.schema.json
  secretSchema: ./secret.schema.json
`, name, name, name)
}

func configSchemaTemplate() string {
	return `{
  "type": "object",
  "properties": {
    "header_name": { "type": "string", "default": "X-Apify-Plugin" },
    "header_value": { "type": "string", "default": "enabled" }
  },
  "additionalProperties": false
}
`
}

func pluginGoModTemplate(name string) string {
	return fmt.Sprintf(`module %s

go 1.26

require github.com/apifyhost/apify-plugin v0.0.0
`, name)
}

func pluginMainTemplate(name string) string {
	return fmt.Sprintf(`package main

import (
	"context"
	"encoding/json"

	sdk "github.com/apifyhost/apify-plugin/plugin"
)

type pluginImpl struct{}

type config struct {
	HeaderName  string `+"`json:\"header_name\"`"+`
	HeaderValue string `+"`json:\"header_value\"`"+`
}

func main() {
	if err := sdk.Serve(pluginImpl{}); err != nil {
		panic(err)
	}
}

func (pluginImpl) Descriptor() sdk.Descriptor {
	return sdk.Descriptor{
		Name:       %q,
		Version:    "1.0.0",
		Runtime:    sdk.RuntimeExternalProcess,
		SDKVersion: ">=0.1.0 <0.2.0",
		Protocol:   "http-json-v1",
		Phases:     []sdk.Phase{sdk.PhaseResponse},
		ConfigSchema: json.RawMessage(`+"`"+`{"type":"object","additionalProperties":true}`+"`"+`),
		Capabilities: []string{"http_response"},
	}
}

func (pluginImpl) ValidateConfig(ctx context.Context, raw json.RawMessage) error {
	var cfg config
	return json.Unmarshal(raw, &cfg)
}

func (pluginImpl) Execute(ctx context.Context, phase sdk.Phase, pc *sdk.Context, raw json.RawMessage) (*sdk.Result, error) {
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
		Headers: map[string]string{cfg.HeaderName: cfg.HeaderValue},
	}, nil
}
`, name)
}
