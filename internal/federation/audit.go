package federation

import (
	"context"
	"fmt"
	"time"
)

// AuditLogger implements audit logging for federation
type AuditLogger struct {
	logs []*AuditLog
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger() *AuditLogger {
	return &AuditLogger{
		logs: []*AuditLog{},
	}
}

// Log logs an audit event
func (a *AuditLogger) Log(ctx context.Context, identity *Identity, action string, resource string, result string) error {
	log := &AuditLog{
		ID:        fmt.Sprintf("audit_%d", time.Now().UnixNano()),
		Timestamp: time.Now(),
		Identity:  identity,
		Action:    action,
		Resource:  resource,
		Result:    result,
		Details:   make(map[string]interface{}),
	}

	a.logs = append(a.logs, log)
	return nil
}

// GetLogs retrieves audit logs
func (a *AuditLogger) GetLogs(ctx context.Context) []*AuditLog {
	return a.logs
}

// GetLogsByIdentity retrieves logs for a specific identity
func (a *AuditLogger) GetLogsByIdentity(ctx context.Context, principal string) []*AuditLog {
	result := []*AuditLog{}
	for _, log := range a.logs {
		if log.Identity != nil && log.Identity.Principal == principal {
			result = append(result, log)
		}
	}
	return result
}
