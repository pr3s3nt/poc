package vault

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"

	configport "orchestrator/internal/ports/configuration"
)

// versionSearch bounds how many earlier KV v2 versions a deploy compares
// before writing a new one (ADR-042).
const versionSearch = 10

// WorkloadValuePath is the fixed KV v2 path of one workload's secret values
// (ADR-042). It stays below the Environment's value prefix, so ownership,
// purge and VSO removal rules apply unchanged.
func WorkloadValuePath(app, env, workload string) string {
	return pathPrefix + app + "/envs/" + env + "/values/workload-" + workload
}

// WorkloadRole is the fixed Vault policy/role name of one workload. The bound
// namespace is part of the role, not of its name, so cleanup needs no target.
func WorkloadRole(app, env, workload string) string {
	sum := sha256.Sum256([]byte("workload-role/v1/" + app + "/" + env + "/" + workload))
	return "orch-wl-" + hex.EncodeToString(sum[:10])
}

// VersionedSecretName is the Kubernetes Secret (and VaultStaticSecret) name of
// one workload's pinned value version.
func VersionedSecretName(workload string, version int) string {
	return workload + "-config-v" + strconv.Itoa(version)
}

// writeVersionedBundle stores values at the workload's fixed path. A content
// equal to the latest or one of the recent versions reuses that version; any
// other content becomes a new version written with check-and-set, so a
// concurrent writer cannot be overwritten silently.
func (p *Provider) writeVersionedBundle(ctx context.Context, app, env, workload, namespace string, values map[string]map[string]string) (configport.WorkloadBundle, error) {
	if !safeSegment.MatchString(app) || !safeSegment.MatchString(workload) || !safeSegment.MatchString(namespace) || (env != "staging" && env != "production") {
		return configport.WorkloadBundle{}, fmt.Errorf("vault: invalid workload bundle scope")
	}
	data, keys, err := bundleData(values)
	if err != nil {
		return configport.WorkloadBundle{}, err
	}
	path := WorkloadValuePath(app, env, workload)
	version, err := p.matchingVersion(ctx, path, data)
	if err != nil {
		return configport.WorkloadBundle{}, err
	}
	if version == 0 {
		version, err = p.writeVersion(ctx, path, data)
		if err != nil {
			return configport.WorkloadBundle{}, err
		}
	}
	role := WorkloadRole(app, env, workload)
	serviceAccount := configport.WorkloadServiceAccount(app, env, workload)
	policy := "path \"" + p.mount + "/data/" + path + "\" { capabilities = [\"read\"] }\n"
	if err := p.writeObject(ctx, "/v1/sys/policies/acl/"+role, map[string]any{"policy": policy}); err != nil {
		return configport.WorkloadBundle{}, err
	}
	if err := p.writeObject(ctx, "/v1/auth/"+p.AuthMount()+"/role/"+role, map[string]any{
		"bound_service_account_names":      serviceAccount,
		"bound_service_account_namespaces": namespace,
		"token_policies":                   role,
		"token_ttl":                        "1h",
	}); err != nil {
		return configport.WorkloadBundle{}, err
	}
	if p.agentAddress == "" {
		return configport.WorkloadBundle{}, fmt.Errorf("vault: in-cluster Agent address is not configured")
	}
	return configport.WorkloadBundle{Address: p.agentAddress, Mount: p.mount, AuthMount: p.AuthMount(), Path: path, Role: role, ServiceAccount: serviceAccount, SecretName: VersionedSecretName(workload, version), CAPEM: p.caPEM, Keys: keys, Version: version}, nil
}

// bundleData flattens container/variable values into Secret data keys.
func bundleData(values map[string]map[string]string) (map[string]string, map[string]map[string]string, error) {
	data := map[string]string{}
	keys := map[string]map[string]string{}
	for container, entries := range values {
		if !safeSegment.MatchString(container) {
			return nil, nil, fmt.Errorf("vault: invalid container name")
		}
		keys[container] = map[string]string{}
		for name, value := range entries {
			if !safeSegment.MatchString(name) {
				return nil, nil, fmt.Errorf("vault: invalid environment variable name")
			}
			key := container + "_" + name
			if len(key) > 253 {
				return nil, nil, fmt.Errorf("vault: configuration key is too long")
			}
			data[key] = value
			keys[container][name] = key
		}
	}
	if len(data) == 0 {
		return nil, nil, fmt.Errorf("vault: no workload configuration references")
	}
	return data, keys, nil
}

type versionRead struct {
	Data struct {
		Data     map[string]string `json:"data"`
		Metadata struct {
			Version      int    `json:"version"`
			DeletionTime string `json:"deletion_time"`
			Destroyed    bool   `json:"destroyed"`
		} `json:"metadata"`
	} `json:"data"`
}

// readVersion returns one readable version; version 0 means the latest.
func (p *Provider) readVersion(ctx context.Context, path string, version int) (versionRead, bool, error) {
	query := ""
	if version > 0 {
		query = "?version=" + strconv.Itoa(version)
	}
	var out versionRead
	status, err := p.request(ctx, http.MethodGet, "/v1/"+p.mount+"/data/"+path+query, nil, &out)
	if err != nil {
		return out, false, err
	}
	switch status {
	case http.StatusOK:
		if out.Data.Metadata.Destroyed || out.Data.Metadata.DeletionTime != "" || out.Data.Data == nil {
			return out, false, nil
		}
		return out, true, nil
	case http.StatusNotFound:
		return out, false, nil
	default:
		return out, false, fmt.Errorf("vault: read workload values status %d", status)
	}
}

// matchingVersion returns the newest readable version among the latest
// versionSearch versions whose data equals data, or zero.
func (p *Provider) matchingVersion(ctx context.Context, path string, data map[string]string) (int, error) {
	latest, ok, err := p.readVersion(ctx, path, 0)
	if err != nil {
		return 0, err
	}
	current := latest.Data.Metadata.Version
	if ok && reflect.DeepEqual(latest.Data.Data, data) {
		return current, nil
	}
	for version := current - 1; version >= 1 && version > current-versionSearch; version-- {
		read, ok, err := p.readVersion(ctx, path, version)
		if err != nil {
			return 0, err
		}
		if ok && reflect.DeepEqual(read.Data.Data, data) {
			return version, nil
		}
	}
	return 0, nil
}

// writeVersion writes data as the next version using check-and-set against the
// latest version, so two writers cannot both claim the same version.
func (p *Provider) writeVersion(ctx context.Context, path string, data map[string]string) (int, error) {
	latest, _, err := p.readVersion(ctx, path, 0)
	if err != nil {
		return 0, err
	}
	body, err := json.Marshal(map[string]any{"options": map[string]any{"cas": latest.Data.Metadata.Version}, "data": data})
	if err != nil {
		return 0, fmt.Errorf("vault: encode workload values")
	}
	var out struct {
		Data struct {
			Version int `json:"version"`
		} `json:"data"`
	}
	status, err := p.request(ctx, http.MethodPost, "/v1/"+p.mount+"/data/"+path, body, &out)
	if err != nil {
		return 0, err
	}
	if status != http.StatusOK || out.Data.Version < 1 {
		return 0, fmt.Errorf("vault: write workload values status %d", status)
	}
	return out.Data.Version, nil
}

// DeleteWorkload removes every version of one workload's value path and its
// fixed policy/role. Absent objects count as deleted.
func (p *Provider) DeleteWorkload(ctx context.Context, app, env, workload string) error {
	if !safeSegment.MatchString(app) || !safeSegment.MatchString(workload) || (env != "staging" && env != "production") {
		return fmt.Errorf("vault: invalid workload scope")
	}
	if err := p.DeleteValue(ctx, "kv2://"+p.mount+"/"+WorkloadValuePath(app, env, workload)); err != nil {
		return err
	}
	role := WorkloadRole(app, env, workload)
	for _, path := range []string{"/v1/auth/" + p.AuthMount() + "/role/" + role, "/v1/sys/policies/acl/" + role} {
		status, err := p.request(ctx, http.MethodDelete, path, nil, nil)
		if err != nil {
			return err
		}
		if status != http.StatusOK && status != http.StatusNoContent && status != http.StatusNotFound {
			return fmt.Errorf("vault: delete workload access status %d", status)
		}
	}
	return nil
}
