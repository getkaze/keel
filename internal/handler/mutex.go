package handler

import (
	"fmt"
	"net/http"
	"sync"
)

// OpMutex prevents concurrent long-running operations.
// Only one operation (install, start, stop, update, reset, deploy) can run at a time.
type OpMutex struct {
	mu     sync.Mutex
	active string // operation type currently running, empty if idle
}

// NewOpMutex creates a new operation mutex.
func NewOpMutex() *OpMutex {
	return &OpMutex{}
}

// TryAcquire attempts to start an operation. Returns a release function on success.
// If another operation is running, writes HTTP 409 and returns nil.
func (m *OpMutex) TryAcquire(w http.ResponseWriter, opType string) func() {
	m.mu.Lock()
	if m.active != "" {
		current := m.active
		m.mu.Unlock()
		http.Error(w,
			fmt.Sprintf("An operation is already in progress: %s", current),
			http.StatusConflict,
		)
		return nil
	}
	m.active = opType
	m.mu.Unlock()

	return func() {
		m.mu.Lock()
		m.active = ""
		m.mu.Unlock()
	}
}

// ActiveOp returns the currently running operation type, or empty string.
func (m *OpMutex) ActiveOp() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active
}
