package envops

import (
	"orchestrator/internal/domain/progress"
	"time"
)

// recordProgress shares the heartbeat writer lock: a heartbeat or Stage may not
// overwrite a more recent diagnostic snapshot. Every write retains owner/fence.
func (l *Lease) recordProgress(step progress.Step, check bool) {
	l.writeMu.Lock()
	defer l.writeMu.Unlock()
	l.mu.Lock()
	if l.ended || l.lost {
		l.mu.Unlock()
		return
	}
	r := progress.Read(l.detail["progress"])
	steps := &r.Steps
	if check {
		steps = &r.Checks
	}
	now := time.Now().UTC()
	step.UpdatedAt = now
	found := false
	for i, old := range *steps {
		if old.ID != step.ID {
			continue
		}
		step.StartedAt = old.StartedAt
		if old.Status == "PENDING" && step.Status == "RUNNING" {
			step.StartedAt = now
		}
		step.Logs = old.Logs
		if step.Stage != "" && (old.Stage != step.Stage || old.Status != step.Status) {
			step.Logs = append(step.Logs, progress.Log{At: now, Stage: step.Stage, Status: step.Status})
			if len(step.Logs) > 100 {
				step.Logs = step.Logs[len(step.Logs)-100:]
			}
		}
		if old.Status == step.Status {
			step.FinishedAt = old.FinishedAt
		}
		if step.Deadline == nil {
			step.Deadline = old.Deadline
		}
		if step.DeploymentID == "" {
			step.DeploymentID = old.DeploymentID
		}
		if step.Pods == nil {
			step.Pods = old.Pods
		}
		if step.Events == nil {
			step.Events = old.Events
		}
		if step.Commit == "" {
			step.Commit = old.Commit
		}
		(*steps)[i] = step
		found = true
		break
	}
	if step.StartedAt.IsZero() {
		step.StartedAt = now
	}
	if step.Status == "SUCCEEDED" || step.Status == "FAILED" || step.Status == "INTERRUPTED" {
		if step.FinishedAt == nil {
			step.FinishedAt = &now
		}
	}
	if !found {
		if step.Stage != "" {
			step.Logs = []progress.Log{{At: now, Stage: step.Stage, Status: step.Status}}
		}
		*steps = append(*steps, step)
	} else {
		for i := range *steps {
			if (*steps)[i].ID == step.ID {
				(*steps)[i] = step
			}
		}
	}
	next := map[string]any{}
	for k, v := range l.detail {
		next[k] = v
	}
	next["progress"] = r
	l.detail = next
	stage := l.stage
	l.mu.Unlock()
	_ = l.heartbeatNow(stage, next)
}

func (l *Lease) PriorHeartbeat(at time.Time) {
	l.writeMu.Lock()
	defer l.writeMu.Unlock()
	l.mu.Lock()
	r := progress.Read(l.detail["progress"])
	if r.PriorHeartbeat == nil {
		r.PriorHeartbeat = &at
	}
	next := map[string]any{}
	for k, v := range l.detail {
		next[k] = v
	}
	next["progress"] = r
	l.detail = next
	stage := l.stage
	l.mu.Unlock()
	_ = l.heartbeatNow(stage, next)
}
func (l *Lease) finishProgress(status string) {
	l.mu.Lock()
	r := progress.Read(l.detail["progress"])
	l.mu.Unlock()
	for _, step := range r.Checks {
		if step.Status == "RUNNING" {
			step.Status = status
			l.recordProgress(step, true)
		}
	}
	for _, step := range r.Steps {
		if step.Status == "RUNNING" {
			step.Status = status
			l.recordProgress(step, false)
		}
	}
}
