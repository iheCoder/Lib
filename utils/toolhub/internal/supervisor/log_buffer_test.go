package supervisor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLogBufferKeepsNewestBytes protects the memory bound while ensuring the UI
// presents the most useful end of a failure log.
func TestLogBufferKeepsNewestBytes(t *testing.T) {
	buffer := NewLogBuffer(5)
	_, _ = buffer.Write([]byte("abc"))
	_, _ = buffer.Write([]byte("defg"))

	content, updated := buffer.Snapshot()
	require.Equal(t, "cdefg", content)
	require.False(t, updated.IsZero())
}
