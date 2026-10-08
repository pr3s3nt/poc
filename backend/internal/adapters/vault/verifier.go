package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"orchestrator/internal/platform/ids"
	configport "orchestrator/internal/ports/configuration"
)

// Verifier proves a Vault KV v2 workload store usable (ADR-012): token valid,
// mount is KV v2, the token holds every capability the product needs
// (value create/read/update/delete plus workload policy and role management)
// and an attempt-owned probe object can be written, read and removed.
type Verifier struct {
	// ProbeTimeout bounds the whole verification.
	ProbeTimeout time.Duration
}

const probeApp = "_verify"

// requiredCapabilities maps a probe path template to the capabilities needed.
func requiredCapabilities(mount, authMount, attempt string) map[string][]string {
	valuePath := mount + "/data/orchestrator/apps/" + probeApp + "/envs/staging/values/" + attempt
	return map[string][]string{
		valuePath: {"create", "read", "update"},
		mount + "/metadata/orchestrator/apps/" + probeApp + "/envs/staging/values/" + attempt: {"delete"},
		"sys/policies/acl/orch-" + attempt:            {"create", "update"},
		"auth/" + authMount + "/role/orch-" + attempt: {"create", "update"},
	}
}

// Verify runs the full verification and always removes its probe object.
func (v Verifier) Verify(ctx context.Context, req configport.VerifyRequest) (map[string]any, error) {
	timeout := v.ProbeTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	p, err := NewWithCA(req.BackendAddress, req.Token, req.Mount, req.CAPEM)
	if err != nil {
		return nil, fmt.Errorf("%w", configport.ErrUnreachable)
	}
	if err := p.SetAuthMount(req.AuthMount); err != nil {
		return nil, configport.ErrUnreachable
	}
	status, err := p.request(ctx, http.MethodGet, "/v1/auth/token/lookup-self", nil, nil)
	if err != nil {
		return nil, configport.ErrUnreachable
	}
	if status != http.StatusOK {
		return nil, configport.ErrTokenRejected
	}
	var mountInfo struct {
		Data struct {
			Type    string            `json:"type"`
			Options map[string]string `json:"options"`
		} `json:"data"`
	}
	status, err = p.request(ctx, http.MethodGet, "/v1/sys/internal/ui/mounts/"+req.Mount, nil, &mountInfo)
	if err != nil {
		return nil, configport.ErrUnreachable
	}
	if status != http.StatusOK || (mountInfo.Data.Type != "kv" && mountInfo.Data.Type != "generic") || mountInfo.Data.Options["version"] != "2" {
		return nil, configport.ErrNotKV2
	}
	attempt := strings.ReplaceAll(ids.New(), "-", "")[:20]
	required := requiredCapabilities(req.Mount, req.AuthMount, attempt)
	paths := make([]string, 0, len(required))
	for path := range required {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	body, _ := json.Marshal(map[string]any{"paths": paths})
	var caps map[string]any
	status, err = p.request(ctx, http.MethodPost, "/v1/sys/capabilities-self", body, &caps)
	if err != nil {
		return nil, configport.ErrUnreachable
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("%w: capabilities could not be read", configport.ErrCapability)
	}
	if missing := missingCapabilities(caps, required); len(missing) > 0 {
		return nil, fmt.Errorf("%w: %s", configport.ErrCapability, strings.Join(missing, ", "))
	}

	probeRef := "kv2://" + req.Mount + "/orchestrator/apps/" + probeApp + "/envs/staging/values/" + attempt
	probeBody, _ := json.Marshal(map[string]any{"options": map[string]any{"cas": 0}, "data": map[string]string{"value": attempt}})
	status, err = p.request(ctx, http.MethodPost, "/v1/"+req.Mount+"/data/orchestrator/apps/"+probeApp+"/envs/staging/values/"+attempt, probeBody, nil)
	// The probe is attempt-owned and always removed, even after a failed write.
	cleanup := func() error {
		cleanCtx, cleanCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cleanCancel()
		return p.DeleteValue(cleanCtx, probeRef)
	}
	if err != nil || (status != http.StatusOK && status != http.StatusNoContent) {
		// A write that failed may still have landed; its removal must be proven.
		if cleanErr := cleanup(); cleanErr != nil {
			return nil, configport.ErrProbeCleanup
		}
		return nil, configport.ErrProbe
	}
	read, readErr := p.ReadValue(ctx, probeRef)
	if cleanErr := cleanup(); cleanErr != nil {
		return nil, configport.ErrProbeCleanup
	}
	if readErr != nil || read != attempt {
		return nil, configport.ErrProbe
	}
	if _, err := p.ReadValue(ctx, probeRef); err == nil {
		return nil, configport.ErrProbeCleanup
	}
	authStatus := "ABSENT"
	if s, err := p.request(ctx, http.MethodGet, "/v1/auth/"+req.AuthMount+"/config", nil, nil); err == nil && s == http.StatusOK {
		authStatus = "CONFIGURED"
	}
	return map[string]any{
		"verified": true, "kvVersion": float64(2), "capabilities": "VERIFIED", "probe": "REMOVED",
		"kubernetesAuth": authStatus, "verifiedAt": time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// missingCapabilities returns the human-readable missing capabilities. A root
// token reports "root". The response may be flat or nested under "data".
func missingCapabilities(response map[string]any, required map[string][]string) []string {
	table := response
	if nested, ok := response["data"].(map[string]any); ok {
		table = nested
	}
	var missing []string
	for path, wanted := range required {
		granted := map[string]bool{}
		if list, ok := table[path].([]any); ok {
			for _, item := range list {
				if text, ok := item.(string); ok {
					granted[text] = true
				}
			}
		}
		if granted["root"] {
			continue
		}
		for _, capability := range wanted {
			if !granted[capability] {
				missing = append(missing, capability+" on "+kindOf(path))
			}
		}
	}
	sort.Strings(missing)
	return missing
}

func kindOf(path string) string {
	switch {
	case strings.Contains(path, "/data/"):
		return "value data path"
	case strings.Contains(path, "/metadata/"):
		return "value metadata path"
	case strings.HasPrefix(path, "sys/policies/"):
		return "workload policies"
	case strings.HasPrefix(path, "auth/"):
		return "Kubernetes auth roles"
	}
	return "path"
}

var _ configport.Verifier = Verifier{}
