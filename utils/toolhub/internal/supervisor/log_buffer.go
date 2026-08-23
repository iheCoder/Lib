package supervisor

import (
	"sync"
	"time"
)

// LogBuffer retains a bounded tail of combined stdout and stderr. A hard byte
// limit prevents noisy tools from growing ToolHub's memory without bound.
type LogBuffer struct {
	mutex    sync.RWMutex
	data     []byte
	capacity int
	updated  time.Time
}

// NewLogBuffer creates a process-safe bounded log sink.
func NewLogBuffer(capacity int) *LogBuffer {
	return &LogBuffer{capacity: capacity}
}

// Write implements io.Writer and always keeps the newest bytes.
func (buffer *LogBuffer) Write(payload []byte) (int, error) {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	buffer.data = append(buffer.data, payload...)
	if overflow := len(buffer.data) - buffer.capacity; overflow > 0 {
		copy(buffer.data, buffer.data[overflow:])
		buffer.data = buffer.data[:buffer.capacity]
	}
	buffer.updated = time.Now()
	return len(payload), nil
}

// Snapshot returns an immutable copy suitable for an API response.
func (buffer *LogBuffer) Snapshot() (string, time.Time) {
	buffer.mutex.RLock()
	defer buffer.mutex.RUnlock()
	return string(append([]byte(nil), buffer.data...)), buffer.updated
}
