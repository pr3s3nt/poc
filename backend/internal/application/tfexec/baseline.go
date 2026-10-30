package tfexec

import (
	"context"
	"encoding/json"
	"fmt"

	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformexec"
	"orchestrator/internal/ports/terraformstate"
)

// baselinePayload never appears in a public report. Variables are the exact
// normalized execution values, including resolved secrets, under State crypto.
type baselinePayload struct {
	Request   ExecuteRequest
	Variables map[string]any
	Outputs   map[string]terraformexec.Output
}

func baselineAAD(a persistence.CipherArtifact) []byte {
	raw, _ := json.Marshal([]any{"reconciliation", a.OrganizationID, a.Purpose, a.OwnerID, a.ID, a.AADVersion})
	return raw
}

func (s *Service) recordBaseline(ctx context.Context, st *execState, out invocationOutcome) error {
	if s.deps.Baselines == nil {
		return nil
	}
	if s.deps.Cipher == nil {
		return fmt.Errorf("%w: private baseline encryption is unavailable", ErrExecution)
	}
	raw, err := json.Marshal(baselinePayload{Request: st.req, Variables: st.vars.All, Outputs: out.outputs})
	if err != nil {
		return fmt.Errorf("%w: private baseline encoding failed", ErrExecution)
	}
	a := persistence.CipherArtifact{ID: ids.New(), OrganizationID: st.inst.Identity.OrganizationID, OwnerID: st.inst.ID, Purpose: "TERRAFORM_BASELINE", KeyID: s.deps.Cipher.KeyID(), AADVersion: 1, CreatedAt: s.now()}
	a.Nonce, a.Ciphertext, err = s.deps.Cipher.Seal(raw, baselineAAD(a))
	if err != nil {
		return fmt.Errorf("%w: private baseline encryption failed", ErrExecution)
	}
	pctx, cancel := persistCtx(ctx)
	defer cancel()
	_, err = s.deps.Baselines.AppendExecutionBaseline(pctx, terraformstate.ExecutionBaseline{ID: ids.New(), OrganizationID: a.OrganizationID, EnvironmentID: st.inst.Identity.EnvironmentID, InstanceID: st.inst.ID, AttemptID: st.attempt.ID, ModuleBundleID: st.bundle.ID, InstanceVersion: st.inst.Version, ArtifactID: a.ID, CreatedAt: a.CreatedAt}, a)
	return err
}
