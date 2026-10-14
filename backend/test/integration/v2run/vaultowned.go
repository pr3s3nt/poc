package v2run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// The secret row uses the EXISTING configuration Vault (read-only reuse of its
// global configuration, auth mount and VSO). The run creates only objects that
// are scoped by its unique Application key: KV values under
// orchestrator/apps/<app>/, per-workload ACL policies that only reference those
// values, and Kubernetes auth roles bound to the run namespace and those
// policies. Nothing else is ever deleted: ownership is proven by content, not
// by name prefix, and objects that existed before the run (the snapshot) are
// never candidates.

// VaultAccess addresses the configuration Vault through the run's scoped token.
type VaultAccess struct {
	Address, Token, Mount, AuthMount string
	Client                           *http.Client
}

// VaultSnapshot lists the policies and roles that existed before the run.
type VaultSnapshot struct {
	Policies []string `json:"policies"`
	Roles    []string `json:"roles"`
	TakenAt  string   `json:"takenAt"`
}

// VaultOwned are the objects of one run found in the Vault.
type VaultOwned struct {
	Values   []string `json:"values"`
	Policies []string `json:"policies"`
	Roles    []string `json:"roles"`
	// Foreign are new objects the run cannot prove to be its own; they are
	// reported and never deleted.
	Foreign []string `json:"foreign,omitempty"`
}

// Empty reports that nothing owned remains.
func (o VaultOwned) Empty() bool { return len(o.Values)+len(o.Policies)+len(o.Roles) == 0 }

// NewVaultAccess reads the scoped token file (never printed).
func NewVaultAccess(s SecretStoreConfig) (VaultAccess, error) {
	if err := requirePrivateFile(s.TokenFile); err != nil {
		return VaultAccess{}, fmt.Errorf("secret store token file: %w", err)
	}
	raw, err := os.ReadFile(s.TokenFile)
	if err != nil {
		return VaultAccess{}, errors.New("secret store token file unreadable")
	}
	tok := strings.TrimSpace(string(raw))
	if tok == "" {
		return VaultAccess{}, errors.New("secret store token file is empty")
	}
	return VaultAccess{Address: strings.TrimRight(s.BackendAddress, "/"), Token: tok, Mount: s.Mount, AuthMount: s.AuthMount,
		Client: &http.Client{Timeout: 20 * time.Second}}, nil
}

func (v VaultAccess) do(ctx context.Context, method, path string, query url.Values) (int, []byte, error) {
	u := v.Address + "/v1/" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return 0, nil, errors.New("vault: build request")
	}
	req.Header.Set("X-Vault-Token", v.Token)
	res, err := v.Client.Do(req)
	if err != nil {
		return 0, nil, errors.New("vault: request failed") // transport errors can quote the URL
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	return res.StatusCode, body, nil
}

func (v VaultAccess) list(ctx context.Context, path string) ([]string, error) {
	code, body, err := v.do(ctx, http.MethodGet, path, url.Values{"list": {"true"}})
	if err != nil {
		return nil, err
	}
	if code == http.StatusNotFound {
		return nil, nil
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("vault: list %s answered %d", path, code)
	}
	var out struct {
		Data struct {
			Keys []string `json:"keys"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, errors.New("vault: list answer unreadable")
	}
	sort.Strings(out.Data.Keys)
	return out.Data.Keys, nil
}

func (v VaultAccess) delete(ctx context.Context, path string) error {
	code, _, err := v.do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	if code != http.StatusNoContent && code != http.StatusOK && code != http.StatusNotFound {
		return fmt.Errorf("vault: delete %s answered %d", path, code)
	}
	return nil
}

// TakeVaultSnapshot records the policies and roles that exist before the run.
func TakeVaultSnapshot(ctx context.Context, v VaultAccess) (VaultSnapshot, error) {
	pol, err := v.list(ctx, "sys/policies/acl")
	if err != nil {
		return VaultSnapshot{}, err
	}
	roles, err := v.list(ctx, "auth/"+v.AuthMount+"/role")
	if err != nil {
		return VaultSnapshot{}, err
	}
	return VaultSnapshot{Policies: pol, Roles: roles, TakenAt: time.Now().UTC().Format(time.RFC3339)}, nil
}

var policyPathLine = regexp.MustCompile(`path\s+"([^"]+)"`)

// valuePaths lists the KV v2 value names (recursively) under the Application prefix.
func (v VaultAccess) valuePaths(ctx context.Context, prefix string) ([]string, error) {
	keys, err := v.list(ctx, v.Mount+"/metadata/"+prefix)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, k := range keys {
		if strings.HasSuffix(k, "/") {
			sub, err := v.valuePaths(ctx, prefix+k)
			if err != nil {
				return nil, err
			}
			out = append(out, sub...)
			continue
		}
		out = append(out, prefix+k)
	}
	return out, nil
}

// FindVaultOwned finds the objects of the run. The Application key is unique
// per run, so everything below its prefix is the run's; a new policy is owned
// only when EVERY path in it lies inside that prefix; a new role is owned only
// when it is bound to exactly the run namespace and carries only owned policies.
func FindVaultOwned(ctx context.Context, v VaultAccess, snap VaultSnapshot, app, namespace string) (VaultOwned, error) {
	var out VaultOwned
	prefix := "orchestrator/apps/" + app + "/"
	values, err := v.valuePaths(ctx, prefix)
	if err != nil {
		return out, err
	}
	out.Values = values
	before := func(list []string) map[string]bool {
		m := map[string]bool{}
		for _, n := range list {
			m[n] = true
		}
		return m
	}
	oldPol, oldRoles := before(snap.Policies), before(snap.Roles)
	pols, err := v.list(ctx, "sys/policies/acl")
	if err != nil {
		return out, err
	}
	ownedPol := map[string]bool{}
	for _, name := range pols {
		if oldPol[name] {
			continue
		}
		code, body, err := v.do(ctx, http.MethodGet, "sys/policies/acl/"+name, nil)
		if err != nil || code != http.StatusOK {
			out.Foreign = append(out.Foreign, "policy:"+name)
			continue
		}
		var doc struct {
			Data struct {
				Policy string `json:"policy"`
			} `json:"data"`
		}
		_ = json.Unmarshal(body, &doc)
		lines := policyPathLine.FindAllStringSubmatch(doc.Data.Policy, -1)
		owned := len(lines) > 0
		for _, l := range lines {
			if !strings.Contains(l[1], "/data/"+prefix) {
				owned = false
			}
		}
		if owned {
			ownedPol[name] = true
			out.Policies = append(out.Policies, name)
		} else {
			out.Foreign = append(out.Foreign, "policy:"+name)
		}
	}
	roles, err := v.list(ctx, "auth/"+v.AuthMount+"/role")
	if err != nil {
		return out, err
	}
	for _, name := range roles {
		if oldRoles[name] {
			continue
		}
		code, body, err := v.do(ctx, http.MethodGet, "auth/"+v.AuthMount+"/role/"+name, nil)
		if err != nil || code != http.StatusOK {
			out.Foreign = append(out.Foreign, "role:"+name)
			continue
		}
		var doc struct {
			Data struct {
				Namespaces []string `json:"bound_service_account_namespaces"`
				Policies   []string `json:"token_policies"`
			} `json:"data"`
		}
		_ = json.Unmarshal(body, &doc)
		owned := len(doc.Data.Namespaces) == 1 && doc.Data.Namespaces[0] == namespace && len(doc.Data.Policies) > 0
		for _, p := range doc.Data.Policies {
			if !ownedPol[p] {
				owned = false
			}
		}
		if owned {
			out.Roles = append(out.Roles, name)
		} else {
			out.Foreign = append(out.Foreign, "role:"+name)
		}
	}
	return out, nil
}

// CleanVaultOwned deletes exactly the owned objects (roles first, then
// policies, then every version of the values) and returns what it removed.
func CleanVaultOwned(ctx context.Context, v VaultAccess, owned VaultOwned) (int, error) {
	n := 0
	for _, r := range owned.Roles {
		if err := v.delete(ctx, "auth/"+v.AuthMount+"/role/"+r); err != nil {
			return n, err
		}
		n++
	}
	for _, p := range owned.Policies {
		if err := v.delete(ctx, "sys/policies/acl/"+p); err != nil {
			return n, err
		}
		n++
	}
	for _, val := range owned.Values {
		if err := v.delete(ctx, v.Mount+"/metadata/"+val); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
