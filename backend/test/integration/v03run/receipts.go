package v03run

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"

	"github.com/jackc/pgx/v5"
)

// Querier is the read-only slice of a pgx connection the receipt readers use.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// ConnectionRef is one Internal account of the run as the product recorded it.
type ConnectionRef struct {
	Organization string `json:"organization"`
	Key          string `json:"key"`
	SecretRef    string `json:"secretRef"`
}

// ReadNamespaceReceipts returns every namespace Terraform instance the product
// recorded in its (run-exclusive, disposable) Console database: the physical
// instance key stamped as the orchestrator-instance label and the Terraform run
// that created it. The Console login may read exactly these columns.
func ReadNamespaceReceipts(ctx context.Context, q Querier) ([]Receipt, error) {
	rows, err := q.Query(ctx, `SELECT descriptor, left(state_key, 16), created_by_run_id FROM terraform_resource_instances WHERE descriptor LIKE 'k8s-namespace.%' ORDER BY descriptor`)
	if err != nil {
		return nil, fmt.Errorf("namespace receipts cannot be read: %w", err)
	}
	defer rows.Close()
	var out []Receipt
	for rows.Next() {
		var r Receipt
		if err := rows.Scan(&r.Descriptor, &r.Instance, &r.TerraformRun); err != nil {
			return nil, fmt.Errorf("namespace receipts cannot be read: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("namespace receipts cannot be read: %w", err)
	}
	return out, nil
}

// ReadConnectionRefs returns the accounts the product stored, with the Vault
// reference of each credential object.
func ReadConnectionRefs(ctx context.Context, q Querier) ([]ConnectionRef, error) {
	rows, err := q.Query(ctx, `SELECT o.organization_key, c.connection_key, c.secret_ref FROM connections c JOIN organizations o ON o.id = c.organization_id ORDER BY o.organization_key, c.connection_key`)
	if err != nil {
		return nil, fmt.Errorf("account receipts cannot be read: %w", err)
	}
	defer rows.Close()
	var out []ConnectionRef
	for rows.Next() {
		var r ConnectionRef
		if err := rows.Scan(&r.Organization, &r.Key, &r.SecretRef); err != nil {
			return nil, fmt.Errorf("account receipts cannot be read: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("account receipts cannot be read: %w", err)
	}
	return out, nil
}

var vaultCredential = regexp.MustCompile(`^kv2://[a-z0-9-]+/orchestrator/connections/([a-z0-9-]+)/([a-z0-9-]+)/credentials/([0-9a-f-]{36})$`)

// VaultKeys returns the exact (organization, account key) pairs of the run: the
// accounts the product stored plus the keys the scenario recorded itself (an
// upload whose row was never committed still names its key there).
func VaultKeys(org string, refs []ConnectionRef, scenarioKeys []string) []string {
	set := map[string]bool{}
	for _, ref := range refs {
		if ref.Organization == org && vaultCredential.MatchString(ref.SecretRef) {
			set[ref.Key] = true
		}
	}
	for _, key := range scenarioKeys {
		if regexp.MustCompile(`^[a-z0-9-]+$`).MatchString(key) {
			set[key] = true
		}
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// AppliedTarget is the safe projection of one persisted workload target: the
// identity the product pinned when it applied the workload. It carries no
// credential; the product never stores one in a target reference.
type AppliedTarget struct {
	WorkloadID               string `json:"workloadId"`
	Status                   string `json:"status"`
	Connection               string `json:"connection"`
	Cluster                  string `json:"cluster"`
	Namespace                string `json:"namespace"`
	Account                  string `json:"account"`
	AccountVersion           int64  `json:"accountVersion"`
	ClusterDefinition        string `json:"clusterDefinition"`
	ClusterDefinitionVersion int64  `json:"clusterDefinitionVersion"`
	ClusterNode              string `json:"clusterNode"`
}

// ReadAppliedTargets reads the persisted target reference of every workload instance.
func ReadAppliedTargets(ctx context.Context, q Querier) ([]AppliedTarget, error) {
	rows, err := q.Query(ctx, `SELECT workload_id, status, target_ref::text FROM workload_instances ORDER BY workload_id`)
	if err != nil {
		return nil, fmt.Errorf("applied targets cannot be read: %w", err)
	}
	defer rows.Close()
	var out []AppliedTarget
	for rows.Next() {
		var id, status, raw string
		if err := rows.Scan(&id, &status, &raw); err != nil {
			return nil, fmt.Errorf("applied targets cannot be read: %w", err)
		}
		target, err := ParseAppliedTarget(id, status, raw)
		if err != nil {
			return nil, err
		}
		out = append(out, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("applied targets cannot be read: %w", err)
	}
	return out, nil
}

// ParseAppliedTarget projects a stored target reference; a missing placement is an error because
// a Definition-placed workload always pins one.
func ParseAppliedTarget(id, status, raw string) (AppliedTarget, error) {
	var ref struct {
		Connection string `json:"connection"`
		Cluster    string `json:"cluster"`
		Namespace  string `json:"namespace"`
		Placement  *struct {
			Account                  string `json:"account"`
			AccountVersion           int64  `json:"accountVersion"`
			ClusterDefinition        string `json:"clusterDefinition"`
			ClusterDefinitionVersion int64  `json:"clusterDefinitionVersion"`
			Cluster                  string `json:"cluster"`
		} `json:"placement"`
	}
	if err := json.Unmarshal([]byte(raw), &ref); err != nil {
		return AppliedTarget{}, fmt.Errorf("the target reference of %s is not readable", id)
	}
	if ref.Placement == nil || ref.Placement.Account == "" || ref.Placement.Cluster == "" {
		return AppliedTarget{}, fmt.Errorf("the target reference of %s pins no placement", id)
	}
	return AppliedTarget{
		WorkloadID: id, Status: status, Connection: ref.Connection, Cluster: ref.Cluster, Namespace: ref.Namespace,
		Account: ref.Placement.Account, AccountVersion: ref.Placement.AccountVersion,
		ClusterDefinition: ref.Placement.ClusterDefinition, ClusterDefinitionVersion: ref.Placement.ClusterDefinitionVersion, ClusterNode: ref.Placement.Cluster,
	}, nil
}
