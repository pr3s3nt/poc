package main

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	connectionapp "orchestrator/internal/application/connection"
)

// fakeKubernetes is a deterministic FAKE Kubernetes verification transport for
// the V01 account scenario (selected by the embedded token prefix). It never
// contacts a cluster and a pass is NOT evidence that a real cluster accepts a
// kubeconfig. It records the endpoint of every check (never a token).
//
//	FAKEK8S-OK*          API reachable and every required permission present
//	FAKEK8S-REJECTED*    the cluster rejects the credential
//	FAKEK8S-NOPERMS*     authenticated but a required permission is missing
//	FAKEK8S-UNREACHABLE* the API cannot be reached
type fakeKubernetes struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeKubernetes) VerifyKubeconfig(_ context.Context, selected connectionapp.SelectedKubeconfig) (connectionapp.KubernetesVerification, error) {
	var doc struct {
		Users []struct {
			User struct {
				Token string `json:"token"`
			} `json:"user"`
		} `json:"users"`
	}
	token := ""
	if json.Unmarshal(selected.Document, &doc) == nil && len(doc.Users) == 1 {
		token = doc.Users[0].User.Token
	}
	f.mu.Lock()
	f.calls = append(f.calls, "verify "+selected.Context.Endpoint)
	f.mu.Unlock()
	switch {
	case strings.HasPrefix(token, "FAKEK8S-OK"):
		return connectionapp.KubernetesVerification{Endpoint: selected.Context.Endpoint, Version: "v1.34.0-fake"}, nil
	case strings.HasPrefix(token, "FAKEK8S-NOPERMS"):
		return connectionapp.KubernetesVerification{}, connectionapp.ErrPermissionDenied
	case strings.HasPrefix(token, "FAKEK8S-UNREACHABLE"):
		return connectionapp.KubernetesVerification{}, connectionapp.ErrClusterUnreachable
	}
	return connectionapp.KubernetesVerification{}, connectionapp.ErrAuthentication
}

func (f *fakeKubernetes) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.calls...)
}
