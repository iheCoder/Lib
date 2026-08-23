package web

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDashboardContainsLifecycleAndNavigationComponents guards the product's
// essential controls against visual-only refactors that remove functionality.
func TestDashboardContainsLifecycleAndNavigationComponents(t *testing.T) {
	html := readFile(t, "index.html")
	javascript := readFile(t, "app.js")

	require.Contains(t, html, `id="tool-list"`)
	require.Contains(t, html, `id="task-dialog"`)
	require.Contains(t, html, `id="logs-dialog"`)
	require.Contains(t, javascript, `link.target = "_blank"`)
	require.Contains(t, javascript, `tool.owned && activeStatuses.has(tool.status)`)
	require.Contains(t, javascript, `if (changed)`)
	require.Contains(t, javascript, `statusLabels`)
}

// TestDashboardHasAccessibilityAndReducedMotion verifies mechanical baseline
// requirements for keyboard, screen-reader, and motion-sensitive users.
func TestDashboardHasAccessibilityAndReducedMotion(t *testing.T) {
	html := readFile(t, "index.html")
	css := readFile(t, "styles.css")

	require.Contains(t, html, `class="skip-link"`)
	require.Contains(t, html, `aria-live="polite"`)
	require.NotContains(t, html, `user-scalable=no`)
	require.Contains(t, css, `prefers-reduced-motion`)
	require.Contains(t, css, `:focus-visible`)
}

// readFile keeps fixture failures tied to the exact missing UI asset.
func readFile(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(name)
	require.NoError(t, err)
	return strings.TrimSpace(string(content))
}
