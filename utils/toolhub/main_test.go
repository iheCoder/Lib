package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestValidateAddressEnforcesLoopback protects the no-authentication boundary
// from a convenient but dangerous future flag change.
func TestValidateAddressEnforcesLoopback(t *testing.T) {
	require.NoError(t, validateAddress("127.0.0.1:17840"))
	require.Error(t, validateAddress("0.0.0.0:17840"))
	require.Error(t, validateAddress("localhost:17840"))
}

// TestLoadEmbeddedCatalog verifies every reviewed registration against the
// schema and confirms cross-project paths remain ToolHub-owned metadata.
func TestLoadEmbeddedCatalog(t *testing.T) {
	registry, err := loadCatalog("", "/tmp/Lib")
	require.NoError(t, err)
	require.Len(t, registry.Tools, 4)
	require.Equal(t, "/tmp/Lib/utils/iina_resume", registry.Tools[0].ResolvedDirectory)
	require.Equal(t, "subtitle-generator", registry.Tools[3].ID)
}
