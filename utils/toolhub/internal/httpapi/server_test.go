package httpapi

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"Lib/utils/toolhub/internal/catalog"
	"Lib/utils/toolhub/internal/supervisor"
	"github.com/stretchr/testify/require"
)

// TestListToolsAndStaticUI verifies API routing wins over the embedded static
// fallback and both surfaces receive the same security headers.
func TestListToolsAndStaticUI(t *testing.T) {
	manager := supervisor.NewManager([]catalog.Tool{{ID: "demo", Name: "Demo", Kind: catalog.KindTask}})
	web := fstest.MapFS{"index.html": {Data: []byte("toolhub")}}
	handler := NewHandler(manager, web)

	api := httptest.NewRecorder()
	handler.ServeHTTP(api, httptest.NewRequest(http.MethodGet, "/api/tools", nil))
	require.Equal(t, http.StatusOK, api.Code)
	require.Contains(t, api.Body.String(), `"id":"demo"`)
	require.Contains(t, api.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'")

	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, page.Code)
	require.Equal(t, "toolhub", page.Body.String())
}

// TestMutationRejectsCrossSiteAndUnknownFields covers the localhost CSRF
// boundary and the strict request contract.
func TestMutationRejectsCrossSiteAndUnknownFields(t *testing.T) {
	handler := testHandler(t)

	crossSite := httptest.NewRequest(http.MethodPost, "/api/tools/demo/start", strings.NewReader(`{}`))
	crossSite.Host = "127.0.0.1:17840"
	crossSite.Header.Set("Content-Type", "application/json")
	crossSite.Header.Set("Origin", "https://malicious.example")
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, crossSite)
	require.Equal(t, http.StatusForbidden, denied.Code)

	unknown := httptest.NewRequest(http.MethodPost, "/api/tools/demo/start", strings.NewReader(`{"unexpected":true}`))
	unknown.Header.Set("Content-Type", "application/json")
	invalid := httptest.NewRecorder()
	handler.ServeHTTP(invalid, unknown)
	require.Equal(t, http.StatusBadRequest, invalid.Code)
}

// TestMissingToolReturnsNotFound ensures domain lookup failures retain useful
// HTTP semantics instead of becoming generic server errors.
func TestMissingToolReturnsNotFound(t *testing.T) {
	manager := supervisor.NewManager([]catalog.Tool{{
		ID: "external", Name: "External", Kind: catalog.KindService,
		URL: "http://127.0.0.1:1", HealthURL: "http://127.0.0.1:1",
	}})
	handler := NewHandler(manager, fstest.MapFS{"index.html": {Data: []byte("ok")}})
	request := httptest.NewRequest(http.MethodPost, "/api/tools/missing/stop", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusNotFound, response.Code)
}

// testHandler builds the smallest valid task manager for request validation.
func testHandler(t *testing.T) http.Handler {
	t.Helper()
	var web fs.FS = fstest.MapFS{"index.html": {Data: []byte("ok")}}
	manager := supervisor.NewManager([]catalog.Tool{{ID: "demo", Name: "Demo", Kind: catalog.KindTask}})
	return NewHandler(manager, web)
}
