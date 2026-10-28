package mcpbridge

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

type auditRecord struct {
	Timestamp   time.Time `json:"timestamp"`
	Tool        string    `json:"tool"`
	Application string    `json:"application,omitempty"`
	Environment string    `json:"environment,omitempty"`
	Confirm     bool      `json:"confirm"`
	Status      int       `json:"status,omitempty"`
	Outcome     string    `json:"outcome"`
}

type auditLogger struct {
	mu     sync.Mutex
	writer io.Writer
	now    func() time.Time
}

func (a *auditLogger) record(record auditRecord) {
	record.Timestamp = a.now().UTC()
	encoded, err := json.Marshal(record)
	if err != nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_, _ = a.writer.Write(append(encoded, '\n'))
}
