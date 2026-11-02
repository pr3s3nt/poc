package fleet

import (
	"context"
	"encoding/json"
	"time"

	k8s "orchestrator/internal/adapters/kubernetes"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/ports/execution"
)

func observeSecret(ctx context.Context, cli k8s.CLI, target execution.Target, object map[string]any) (domain.Health, error) {
	md, _ := object["metadata"].(map[string]any)
	spec, _ := object["spec"].(map[string]any)
	destination, _ := spec["destination"].(map[string]any)
	name, _ := destination["name"].(string)
	uid, _ := md["uid"].(string)
	annotations, _ := md["annotations"].(map[string]any)
	rawKeys, _ := annotations["orchestrator.io/secret-keys"].(string)
	var keys []string
	if name == "" || name != md["name"] || uid == "" || json.Unmarshal([]byte(rawKeys), &keys) != nil || len(keys) == 0 {
		return domain.HealthUnknown, ErrConfig
	}
	secret, err := k8s.ReadSecretShape(ctx, cli, target.Namespace, name)
	if err != nil {
		return domain.HealthUnknown, err
	}
	if secret.UID == "" {
		return domain.Progressing, nil
	}
	if !secret.OwnedBy(uid) {
		return domain.HealthUnknown, ErrOwnership
	}
	for _, key := range keys {
		if !secret.Keys[key] {
			return domain.Progressing, nil
		}
	}
	return domain.Healthy, nil
}

// VSO owns its generated Secret. Wait for Kubernetes garbage collection rather
// than putting secret bytes/children into desired Git inventory or deleting by name.
func waitSecretRemoved(ctx context.Context, cli k8s.CLI, target execution.Target, name, ownerUID string) error {
	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	for {
		secret, err := k8s.ReadSecretShape(bounded, cli, target.Namespace, name)
		if err != nil {
			return err
		}
		if secret.UID == "" {
			return nil
		}
		if !secret.OwnedBy(ownerUID) {
			return ErrOwnership
		}
		select {
		case <-bounded.Done():
			return ErrController
		case <-time.After(time.Second):
		}
	}
}
