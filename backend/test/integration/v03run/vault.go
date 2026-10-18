package v03run

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// Vault is the minimal strict client the runner's cleanup uses. Every response
// is checked: a network error, an unexpected status or a malformed body is a
// failure, never "nothing there".
type Vault struct {
	Address string
	Mount   string
	Token   string
	Client  *http.Client
}

var (
	accountKey = regexp.MustCompile(`^[a-z0-9-]+$`)
	objectID   = regexp.MustCompile(`^[0-9a-f-]{36}$`)
)

func (v Vault) do(ctx context.Context, method, path string) (int, []byte, error) {
	client := v.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(v.Address, "/")+"/v1/"+v.Mount+"/"+path, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("X-Vault-Token", v.Token)
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("vault request failed")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, nil, fmt.Errorf("vault response unreadable")
	}
	return resp.StatusCode, body, nil
}

// ListObjects returns the credential object IDs under one account key. A 404 is
// the only proof of "none"; anything else that is not a well-formed 200 fails.
func (v Vault) ListObjects(ctx context.Context, org, key string) ([]string, error) {
	if !accountKey.MatchString(org) || !accountKey.MatchString(key) {
		return nil, fmt.Errorf("account key %q is not an acceptable path segment", key)
	}
	status, body, err := v.do(ctx, "LIST", "metadata/orchestrator/connections/"+org+"/"+key+"/credentials")
	if err != nil {
		return nil, err
	}
	switch status {
	case http.StatusNotFound:
		return nil, nil
	case http.StatusOK:
		var doc struct {
			Data struct {
				Keys []string `json:"keys"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &doc) != nil || doc.Data.Keys == nil {
			return nil, fmt.Errorf("vault list of %s returned a malformed body", key)
		}
		var ids []string
		for _, k := range doc.Data.Keys {
			if !objectID.MatchString(k) {
				return nil, fmt.Errorf("vault list of %s returned an unexpected entry", key)
			}
			ids = append(ids, k)
		}
		return ids, nil
	default:
		return nil, fmt.Errorf("vault list of %s failed with status %d", key, status)
	}
}

// DeleteObject removes one credential object (all versions) and proves it absent.
func (v Vault) DeleteObject(ctx context.Context, org, key, id string) error {
	if !accountKey.MatchString(org) || !accountKey.MatchString(key) || !objectID.MatchString(id) {
		return fmt.Errorf("credential path is not acceptable")
	}
	status, _, err := v.do(ctx, http.MethodDelete, "metadata/orchestrator/connections/"+org+"/"+key+"/credentials/"+id)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusOK && status != http.StatusNotFound {
		return fmt.Errorf("vault delete of %s/%s failed with status %d", key, id, status)
	}
	status, _, err = v.do(ctx, http.MethodGet, "data/orchestrator/connections/"+org+"/"+key+"/credentials/"+id)
	if err != nil {
		return err
	}
	if status != http.StatusNotFound {
		return fmt.Errorf("vault object %s/%s is not proven absent (status %d)", key, id, status)
	}
	return nil
}

// CleanResult reports what was removed and what could not be proven.
type CleanResult struct {
	Deleted []string `json:"deleted"`
	Failed  []string `json:"failed"`
}

// CleanAccounts deletes every credential object under the exact account keys of
// the run and verifies each absent; a key whose listing fails is a failure.
func (v Vault) CleanAccounts(ctx context.Context, org string, keys []string) CleanResult {
	var result CleanResult
	for _, key := range keys {
		ids, err := v.ListObjects(ctx, org, key)
		if err != nil {
			result.Failed = append(result.Failed, key+": "+err.Error())
			continue
		}
		for _, id := range ids {
			if err := v.DeleteObject(ctx, org, key, id); err != nil {
				result.Failed = append(result.Failed, key+"/"+id+": "+err.Error())
				continue
			}
			result.Deleted = append(result.Deleted, key+"/"+id)
		}
		// Prove the key is empty after the deletes.
		left, err := v.ListObjects(ctx, org, key)
		if err != nil {
			result.Failed = append(result.Failed, key+": "+err.Error())
		} else if len(left) > 0 {
			result.Failed = append(result.Failed, fmt.Sprintf("%s: %d object(s) remain", key, len(left)))
		}
	}
	return result
}
