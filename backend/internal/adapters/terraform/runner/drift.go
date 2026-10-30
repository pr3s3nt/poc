package runner

import (
	"context"
	"io"
	"orchestrator/internal/domain/drift"
	"orchestrator/internal/ports/terraformexec"
	"os"
	"syscall"
)

var _ terraformexec.DriftPlanSession = (*session)(nil)

// InspectDrift reads only an already saved plan. It cannot refresh/adopt state,
// apply a plan or invoke another planning mode as a side effect of this read.
func (s *session) InspectDrift(ctx context.Context) (drift.Report, error) {
	out, err := s.step(ctx, "inspect-drift-plan", 32<<20, "show", "-json", "-no-color", s.planPath())
	if err != nil {
		return drift.Report{}, err
	}
	report, err := drift.Parse(out)
	if err != nil {
		return drift.Report{}, &terraformexec.StepError{Step: "inspect-drift-plan", Class: terraformexec.ClassInput, Outcome: terraformexec.OutcomeConfirmed, Stopped: true}
	}
	return report, nil
}
func (s *session) ReadSavedPlan(ctx context.Context) ([]byte, error) {
	if ctx.Err() != nil || s.verify() != nil {
		return nil, terraformexec.ErrWorkspace
	}
	file, err := os.OpenFile(s.planPath(), os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, terraformexec.ErrWorkspace
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 32<<20 {
		return nil, terraformexec.ErrWorkspace
	}
	// Terraform's generated plan can have default file mode 0644 within the
	// existing 0700 workspace. Tighten the same open regular file before export.
	if file.Chmod(0o600) != nil {
		return nil, terraformexec.ErrWorkspace
	}
	body, err := io.ReadAll(io.LimitReader(file, 32<<20+1))
	if err != nil || int64(len(body)) != info.Size() {
		return nil, terraformexec.ErrWorkspace
	}
	return body, nil
}
