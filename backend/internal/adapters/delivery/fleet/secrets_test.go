package fleet

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	k8s "orchestrator/internal/adapters/kubernetes"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/ports/execution"
)

func TestVSOPrerequisiteWaitsForOwnedSecretAndEveryRequiredKey(t *testing.T) {
	object := map[string]any{
		"metadata": map[string]any{"name": "orch-bundle", "uid": "vso-uid", "annotations": map[string]any{"orchestrator.io/secret-keys": `["main_PASSWORD","main_TOKEN"]`}},
		"spec":     map[string]any{"destination": map[string]any{"name": "orch-bundle"}},
	}
	for _, tc := range []struct {
		name, output string
		health       domain.Health
		err          error
	}{
		{"not created", "", domain.Progressing, nil},
		{"missing key", "secret-uid\nvso-uid\n--keys--\nmain_PASSWORD\n", domain.Progressing, nil},
		{"complete", "secret-uid\nvso-uid\n--keys--\nmain_PASSWORD\nmain_TOKEN\n", domain.Healthy, nil},
		{"foreign secret", "secret-uid\nforeign-vso\n--keys--\nmain_PASSWORD\nmain_TOKEN\n", domain.HealthUnknown, ErrOwnership},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			output := filepath.Join(dir, "output")
			if err := os.WriteFile(output, []byte(tc.output), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "synthetic-kubeconfig"), []byte("synthetic"), 0600); err != nil {
				t.Fatal(err)
			}
			command := filepath.Join(dir, "kubectl")
			if err := os.WriteFile(command, []byte("#!/bin/sh\ncat '"+output+"'\n"), 0700); err != nil {
				t.Fatal(err)
			}
			health, err := observeSecret(context.Background(), k8s.CLI{Path: command, Kubeconfig: filepath.Join(dir, "synthetic-kubeconfig")}, execution.Target{Namespace: "app"}, object)
			if health != tc.health || !errors.Is(err, tc.err) {
				t.Fatalf("prerequisite gate: health=%s err=%v", health, err)
			}
		})
	}
}
