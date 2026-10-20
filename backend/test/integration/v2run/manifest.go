package v2run

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"orchestrator/internal/domain/environment"
)

// Asset ownership.
const (
	OwnerCreatedByRun = "CREATED_BY_RUN"
	OwnerPreexisting  = "PREEXISTING"
	OwnerRetained     = "RETAINED"
)

// Asset is one physical object the run created, found or retained.
type Asset struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Layer string `json:"layer"`
	Owner string `json:"owner"`
	// UID is the immutable creation identity of a Kubernetes object.
	UID string `json:"uid,omitempty"`
	// Linkage is the exact owner chain of a derived asset (for example the
	// VPC ID or cluster name the asset hangs off). Cleanup never matches by
	// name prefix: only by exact ID or exact linkage.
	Linkage    string    `json:"linkage,omitempty"`
	ObservedAt time.Time `json:"observedAt"`
}

// Planned is a resource count recorded before the first create.
type Planned struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

// Pin is a recorded tool, provider or module pin.
type Pin struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
}

// Manifest is the private run ownership manifest. It is rewritten atomically
// under an exclusive lock so the runner, the supervisor tooling and helpers
// can append safely.
type Manifest struct {
	RunID     string            `json:"runId"`
	Target    string            `json:"target"`
	Scenario  string            `json:"scenario"`
	Account   string            `json:"account,omitempty"`
	Region    string            `json:"region,omitempty"`
	Principal string            `json:"principal,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
	ExpiresAt time.Time         `json:"expiresAt"`
	Tags      map[string]string `json:"tags"`
	Pins      []Pin             `json:"pins"`
	Planned   []Planned         `json:"planned"`
	// Armed is set once pins, counts and expiry are recorded and the
	// supervisor is running; Planned and Pins are immutable afterwards.
	Armed bool `json:"armed"`
	// Namespace is the exact resulting physical namespace read from the
	// Environment (generation suffix included), never a guessed name.
	Namespace         string `json:"namespace,omitempty"`
	NamespaceIdentity string `json:"namespaceIdentity,omitempty"`
	Generation        int64  `json:"generation,omitempty"`
	// TerraformRuns are the Terraform run IDs the run deploys with (the
	// Preview's run ID), recorded BEFORE the Deploy click. They are the only
	// authority of the safety net over Terraform instances.
	TerraformRuns []string `json:"terraformRuns,omitempty"`
	Assets        []Asset  `json:"assets"`
	SentinelUID   string   `json:"sentinelUid,omitempty"`
}

// NewManifest builds the pre-create manifest of a validated configuration.
func NewManifest(c Config, now time.Time) Manifest {
	m := Manifest{RunID: c.RunID, Target: c.Target, Scenario: c.Scenario, CreatedAt: now.UTC(),
		ExpiresAt: now.Add(c.Execution.Duration).UTC(), Tags: c.Tags(now), Pins: []Pin{}, Planned: []Planned{}, Assets: []Asset{}}
	if c.AWS != nil {
		m.Account, m.Region, m.Principal = c.AWS.AccountID, c.AWS.Region, c.AWS.PrincipalARN
	}
	if c.Kube != nil {
		m.SentinelUID = c.Kube.Sentinel.UID
	}
	return m
}

// RecordNamespace records the Environment's exact physical namespace. It must
// equal NamespaceFor(identity, generation) and, on k8s-4f, carry the
// orch-v2-test-<run> prefix; anything else is refused so cleanup can never be
// derived from a guessed name.
func (m *Manifest) RecordNamespace(identity string, generation int64, observed string) error {
	if want := environment.NamespaceFor(identity, generation); want != observed {
		return fmt.Errorf("manifest: the Environment reports namespace %q but identity %q generation %d derives %q", observed, identity, generation, want)
	}
	if m.Target == TargetK8s4f && !strings.HasPrefix(observed, NamespacePrefix+m.RunID) {
		return fmt.Errorf("manifest: namespace %q is not run-owned (%s%s*)", observed, NamespacePrefix, m.RunID)
	}
	if m.Namespace != "" && m.Namespace != observed {
		return fmt.Errorf("manifest: namespace already recorded as %q", m.Namespace)
	}
	m.Namespace, m.NamespaceIdentity, m.Generation = observed, identity, generation
	return nil
}

var terraformRunPattern = regexp.MustCompile(`^pending-[0-9a-f]{16}$`)

// AddTerraformRun records a Terraform run ID of this run (idempotent). Only the
// Preview's `pending-<hash>` identifier is accepted.
func (m *Manifest) AddTerraformRun(id string) error {
	if !terraformRunPattern.MatchString(id) {
		return fmt.Errorf("manifest: %q is not a Preview run id", id)
	}
	for _, have := range m.TerraformRuns {
		if have == id {
			return nil
		}
	}
	m.TerraformRuns = append(m.TerraformRuns, id)
	return nil
}

// Arm freezes pins, counts and expiry. It fails without them.
func (m *Manifest) Arm() error {
	switch {
	case m.Armed:
		return errors.New("manifest: already armed")
	case len(m.Pins) == 0 || len(m.Planned) == 0 || m.ExpiresAt.IsZero():
		return errors.New("manifest: pins, planned resource counts and expiry must be recorded before the first create")
	}
	for _, key := range []string{"project", "owner", "environment", "run-id", "expires-at", "managed-by"} {
		if m.Tags[key] == "" {
			return fmt.Errorf("manifest: tag %q is missing", key)
		}
	}
	m.Armed = true
	return nil
}

// AddPlanned records a planned count (before arming only).
func (m *Manifest) AddPlanned(kind string, count int) error {
	if m.Armed {
		return errors.New("manifest: planned counts are immutable once armed")
	}
	m.Planned = append(m.Planned, Planned{Kind: kind, Count: count})
	return nil
}

// AddPin records a pin (before arming only).
func (m *Manifest) AddPin(p Pin) error {
	if m.Armed {
		return errors.New("manifest: pins are immutable once armed")
	}
	m.Pins = append(m.Pins, p)
	return nil
}

// AddAsset records an observed asset. Ownership never upgrades: a PREEXISTING
// or RETAINED asset cannot become CREATED_BY_RUN, so it can never reach a
// delete step.
func (m *Manifest) AddAsset(a Asset) error {
	if a.Kind == "" || a.ID == "" || a.Layer == "" {
		return errors.New("manifest: asset kind, id and layer are required")
	}
	if _, ok := layerOrder[a.Layer]; !ok {
		return fmt.Errorf("manifest: unknown layer %q", a.Layer)
	}
	switch a.Owner {
	case OwnerCreatedByRun, OwnerPreexisting, OwnerRetained:
	default:
		return fmt.Errorf("manifest: unknown owner %q", a.Owner)
	}
	for i, have := range m.Assets {
		if have.Kind == a.Kind && have.ID == a.ID {
			if have.Owner != a.Owner && (have.Owner != OwnerCreatedByRun || a.Owner == OwnerCreatedByRun) {
				return fmt.Errorf("manifest: asset %s/%s ownership %s cannot change to %s", a.Kind, a.ID, have.Owner, a.Owner)
			}
			if have.Linkage != "" && a.Linkage != "" && have.Linkage != a.Linkage {
				return fmt.Errorf("manifest: asset %s/%s linkage cannot change", a.Kind, a.ID)
			}
			if have.UID != "" && a.UID != "" && have.UID != a.UID {
				return fmt.Errorf("manifest: asset %s/%s UID cannot change", a.Kind, a.ID)
			}
			if have.UID == "" {
				m.Assets[i].UID = a.UID
			}
			if have.Linkage == "" && a.Linkage != "" {
				m.Assets[i].Linkage = a.Linkage
			}
			m.Assets[i].Owner = a.Owner // CREATED_BY_RUN may be downgraded to RETAINED (user kept it), never the reverse
			return nil
		}
	}
	if a.ObservedAt.IsZero() {
		a.ObservedAt = time.Now().UTC()
	}
	m.Assets = append(m.Assets, a)
	return nil
}

// OwnedByRun lists the assets that cleanup may delete, in reverse dependency
// order (delivery first, VPC/aux last), each layer sorted by kind and ID.
func (m Manifest) OwnedByRun() []Asset {
	var out []Asset
	for _, a := range m.Assets {
		if a.Owner == OwnerCreatedByRun {
			out = append(out, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if li, lj := layerOrder[out[i].Layer], layerOrder[out[j].Layer]; li != lj {
			return li < lj
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Protected lists the assets that must be proven still present afterwards.
func (m Manifest) Protected() []Asset {
	var out []Asset
	for _, a := range m.Assets {
		if a.Owner != OwnerCreatedByRun {
			out = append(out, a)
		}
	}
	return out
}

// Public is the safe evidence view: counts, kinds and IDs; no handles.
func (m Manifest) Public() Manifest { return m }

// LoadManifest reads a private manifest.
func LoadManifest(path string) (Manifest, error) {
	var m Manifest
	if err := requirePrivateFile(path); err != nil {
		return m, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(raw, &m)
}

// SaveManifest creates the manifest (it must not exist).
func SaveManifest(path string, m Manifest) error {
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("manifest %s already exists", filepath.Base(path))
	}
	return writeJSONPrivate(path, m)
}

// UpdateManifest applies fn to the manifest under an exclusive lock and
// writes the result atomically; a failing fn writes nothing.
func UpdateManifest(path string, fn func(*Manifest) error) error {
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	m, err := LoadManifest(path)
	if err != nil {
		return err
	}
	if err := fn(&m); err != nil {
		return err
	}
	return writeJSONPrivate(path, m)
}
