package v2run_test

import (
	"encoding/json"

	"orchestrator/test/integration/v2run"
)

func writeSpec(path string, spec v2run.Spec) error {
	raw, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	return v2run.WriteFilePrivate(path, raw)
}

func writeSpecAny(path string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return v2run.WriteFilePrivate(path, raw)
}
