package catalog

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestLoadExpandsRepositoryRoot verifies that managed tools remain relocatable
// even though the central catalog owns all registration metadata.
func TestLoadExpandsRepositoryRoot(t *testing.T) {
	registry, err := Load(strings.NewReader(`
version: 1
tools:
  - id: demo
    name: Demo
    kind: service
    working_directory: ${REPO_ROOT}/utils/demo
    command: go
    args: [run, .]
    url: http://127.0.0.1:19001
    health_url: http://127.0.0.1:19001/health
    startup_timeout: 3s
    stop_timeout: 2s
`), "/tmp/repository")

	require.NoError(t, err)
	require.Equal(t, "/tmp/repository/utils/demo", registry.Tools[0].ResolvedDirectory)
	require.Equal(t, 3*time.Second, registry.Tools[0].StartupTimeout.Duration)
}

// TestLoadRejectsNonLocalService prevents a catalog edit from accidentally
// turning ToolHub into a launcher for network-exposed management endpoints.
func TestLoadRejectsNonLocalService(t *testing.T) {
	_, err := Load(strings.NewReader(`
version: 1
tools:
  - id: unsafe
    name: Unsafe
    kind: service
    working_directory: /tmp
    command: demo
    url: http://0.0.0.0:9000
    health_url: http://0.0.0.0:9000
`), "/tmp/repository")

	require.ErrorContains(t, err, "must use http://127.0.0.1")
}

// TestLoadRejectsUnknownFields catches misspelled registration keys so coding
// agents cannot believe a setting is active when it was actually ignored.
func TestLoadRejectsUnknownFields(t *testing.T) {
	_, err := Load(strings.NewReader("version: 1\ntoolz: []\n"), "/tmp/repository")
	require.ErrorContains(t, err, "field toolz not found")
}
