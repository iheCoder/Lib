// Package catalog loads the ToolHub-owned registry of local tools.
//
// Tool definitions intentionally live inside ToolHub rather than beside each
// managed project. This keeps integration knowledge out of unrelated tools and
// gives humans and coding agents one reviewable place to add registrations.
package catalog

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	KindService = "service"
	KindTask    = "task"
)

var validID = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Registry is the versioned root of the central tool catalog.
type Registry struct {
	Version int    `yaml:"version"`
	Tools   []Tool `yaml:"tools"`
}

// Tool describes how ToolHub starts a process and presents it to the user.
// Command and Args are passed directly to exec.Command; no shell is involved.
type Tool struct {
	ID                string            `yaml:"id"`
	Name              string            `yaml:"name"`
	Description       string            `yaml:"description"`
	Category          string            `yaml:"category"`
	Kind              string            `yaml:"kind"`
	WorkingDirectory  string            `yaml:"working_directory"`
	Command           string            `yaml:"command"`
	Args              []string          `yaml:"args"`
	Environment       map[string]string `yaml:"environment"`
	URL               string            `yaml:"url"`
	HealthURL         string            `yaml:"health_url"`
	StartupTimeout    Duration          `yaml:"startup_timeout"`
	StopTimeout       Duration          `yaml:"stop_timeout"`
	Inputs            []Input           `yaml:"inputs"`
	ResolvedDirectory string            `yaml:"-"`
}

// Input defines one safe, structured task argument rendered by the web UI.
// Named inputs append Flag + value, positional inputs append only the value,
// and boolean inputs append Flag only when selected.
type Input struct {
	ID          string   `yaml:"id" json:"id"`
	Label       string   `yaml:"label" json:"label"`
	Type        string   `yaml:"type" json:"type"`
	Flag        string   `yaml:"flag" json:"flag,omitempty"`
	Position    bool     `yaml:"position" json:"position,omitempty"`
	Required    bool     `yaml:"required" json:"required,omitempty"`
	Default     string   `yaml:"default" json:"default,omitempty"`
	Placeholder string   `yaml:"placeholder" json:"placeholder,omitempty"`
	Options     []string `yaml:"options" json:"options,omitempty"`
}

// Duration gives catalog authors readable values such as "5s" while keeping
// runtime code strongly typed.
type Duration struct {
	time.Duration
}

// UnmarshalYAML parses a duration and rejects ambiguous numeric values.
func (duration *Duration) UnmarshalYAML(node *yaml.Node) error {
	parsed, err := time.ParseDuration(strings.TrimSpace(node.Value))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", node.Value, err)
	}
	duration.Duration = parsed
	return nil
}

// Load decodes, expands, and validates a complete central registry.
func Load(reader io.Reader, repositoryRoot string) (Registry, error) {
	var registry Registry
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)
	if err := decoder.Decode(&registry); err != nil {
		return Registry{}, fmt.Errorf("decode tool catalog: %w", err)
	}
	if err := resolveAndValidate(&registry, repositoryRoot); err != nil {
		return Registry{}, err
	}
	return registry, nil
}

// resolveAndValidate makes filesystem assumptions explicit before any command
// can reach the process launcher.
func resolveAndValidate(registry *Registry, repositoryRoot string) error {
	if registry.Version != 1 {
		return fmt.Errorf("unsupported catalog version %d", registry.Version)
	}
	seen := make(map[string]struct{}, len(registry.Tools))
	for index := range registry.Tools {
		tool := &registry.Tools[index]
		tool.ResolvedDirectory = expandDirectory(tool.WorkingDirectory, repositoryRoot)
		if err := validateTool(*tool, seen); err != nil {
			return fmt.Errorf("tool %q: %w", tool.ID, err)
		}
		seen[tool.ID] = struct{}{}
	}
	return nil
}

// validateTool rejects unsafe or incomplete registrations at startup rather
// than turning them into confusing failures after the user clicks Run.
func validateTool(tool Tool, seen map[string]struct{}) error {
	if tool.ID == "" || tool.Name == "" || tool.Command == "" {
		return errors.New("id, name, and command are required")
	}
	if !validID.MatchString(tool.ID) {
		return errors.New("id must use lowercase letters, numbers, and hyphens")
	}
	if _, duplicate := seen[tool.ID]; duplicate {
		return errors.New("duplicate id")
	}
	if tool.Kind != KindService && tool.Kind != KindTask {
		return fmt.Errorf("kind must be %q or %q", KindService, KindTask)
	}
	if tool.ResolvedDirectory == "" || !filepath.IsAbs(tool.ResolvedDirectory) {
		return errors.New("working_directory must resolve to an absolute path")
	}
	if tool.StartupTimeout.Duration < 0 || tool.StopTimeout.Duration < 0 {
		return errors.New("timeouts cannot be negative")
	}
	if tool.Kind == KindService {
		return validateService(tool)
	}
	return validateInputs(tool.Inputs)
}

// validateService enforces the URL contract required for external-instance
// discovery and safe navigation from the dashboard.
func validateService(tool Tool) error {
	if tool.URL == "" || tool.HealthURL == "" {
		return errors.New("services require url and health_url")
	}
	for _, rawURL := range []string{tool.URL, tool.HealthURL} {
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" {
			return fmt.Errorf("service URL %q must use http://127.0.0.1", rawURL)
		}
	}
	return nil
}

// validateInputs keeps task argument construction deterministic and prevents
// duplicate fields from silently overwriting one another.
func validateInputs(inputs []Input) error {
	seen := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		if input.ID == "" || input.Label == "" {
			return errors.New("each input requires id and label")
		}
		if _, duplicate := seen[input.ID]; duplicate {
			return fmt.Errorf("duplicate input %q", input.ID)
		}
		if input.Type != "text" && input.Type != "select" && input.Type != "boolean" {
			return fmt.Errorf("input %q has unsupported type %q", input.ID, input.Type)
		}
		if !input.Position && input.Flag == "" {
			return fmt.Errorf("input %q requires flag or position", input.ID)
		}
		if input.Type == "select" && (len(input.Options) == 0 || (input.Default != "" && !stringSliceContains(input.Options, input.Default))) {
			return fmt.Errorf("select input %q requires options containing its default", input.ID)
		}
		if input.Type == "boolean" && input.Default != "" && input.Default != "true" && input.Default != "false" {
			return fmt.Errorf("boolean input %q has invalid default", input.ID)
		}
		seen[input.ID] = struct{}{}
	}
	return nil
}

// stringSliceContains keeps enum validation local without introducing a
// generic abstraction for one concrete catalog rule.
func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// expandDirectory supports relocatable in-repository tools and explicit
// cross-project paths without requiring either managed project to know ToolHub.
func expandDirectory(value, repositoryRoot string) string {
	expanded := strings.ReplaceAll(value, "${REPO_ROOT}", repositoryRoot)
	if strings.HasPrefix(expanded, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			expanded = filepath.Join(home, strings.TrimPrefix(expanded, "~/"))
		}
	}
	return filepath.Clean(expanded)
}
