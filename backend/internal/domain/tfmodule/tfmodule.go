// Package tfmodule holds the Terraform v2 module registry model: the exact
// source pin a Resource Definition references, the verified bundle contract
// the registry publishes after inspection and the strict Terraform Driver
// Inputs shape (Terraform module contract, UC-03 BR-09/BR-11).
//
// The model carries no credential or cache path. Direct Git bundles retain
// their validated, credential-free HTTPS origin in the inspected contract.
package tfmodule

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Status of a verified bundle. Bundles are insert-only; there is no revoke
// flow in this iteration.
const StatusVerified = "VERIFIED"

// Source kinds of the Admin/platform source policy.
const (
	SourceKindPackaged = "packaged"
	SourceKindGit      = "git"
	SourceKindZIP      = "zip"
)

// Provider binding kinds a release provider declares (module contract
// "Provider bindings và dependency").
const (
	BindingAWS        = "aws"
	BindingKubernetes = "kubernetes"
	BindingNone       = "none"
)

// Provider binding targets a Definition may declare.
const (
	TargetResource = "resource"
	// TargetAccount binds an AWS provider to the Definition's Driver Account.
	TargetAccount = "account"
)

var (
	identifierPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	revisionPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	commitPattern     = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
	digestPattern     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	segmentPattern    = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._-]*$`)
	versionPattern    = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
)

// ValidVersion reports whether value is an exact MAJOR.MINOR.PATCH version.
func ValidVersion(value string) bool { return versionPattern.MatchString(value) }

// ValidIdentifier reports whether value is a source or module identifier.
func ValidIdentifier(value string) bool { return identifierPattern.MatchString(value) }

// ValidDigest reports whether value is a lowercase hex SHA-256 digest.
func ValidDigest(value string) bool { return digestPattern.MatchString(value) }

// FullCommit reports whether revision is a full Git commit ID (SHA-1 or
// SHA-256 object format). Branches, tags and abbreviated IDs are not.
func FullCommit(revision string) bool { return commitPattern.MatchString(revision) }

// ValidSubdirectory reports whether path is "." or a clean relative slash
// path without empty, "." or ".." segments.
func ValidSubdirectory(path string) bool {
	if path == "." {
		return true
	}
	if path == "" || len(path) > 512 {
		return false
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "." || segment == ".." || !segmentPattern.MatchString(segment) {
			return false
		}
	}
	return true
}

// SourceRef is the exact module pin of a Terraform Definition:
// `driverInputs.values.source`.
type SourceRef struct {
	SourceID      string `json:"source_id"`
	ModuleID      string `json:"module_id"`
	Revision      string `json:"revision"`
	Subdirectory  string `json:"subdirectory"`
	ContentSHA256 string `json:"content_sha256"`
}

// sourceRefFields are the exact keys of a submitted source object.
var sourceRefFields = []string{"source_id", "module_id", "revision", "subdirectory", "content_sha256"}

// Validate checks the shape of every field without consulting any policy.
// Errors name the field, never the value.
func (r SourceRef) Validate() error {
	switch {
	case !ValidIdentifier(r.SourceID):
		return errors.New("source.source_id must be a lowercase source identifier")
	case !ValidIdentifier(r.ModuleID):
		return errors.New("source.module_id must be a lowercase module identifier")
	case !revisionPattern.MatchString(r.Revision):
		return errors.New("source.revision must be an exact revision")
	case !ValidSubdirectory(r.Subdirectory):
		return errors.New("source.subdirectory must be \".\" or a clean relative path")
	case !ValidDigest(r.ContentSHA256):
		return errors.New("source.content_sha256 must be a lowercase hex SHA-256 digest")
	}
	return nil
}

// Pin identifies one revision path; a pin has exactly one verified digest.
func (r SourceRef) Pin() string {
	return r.SourceID + "\x00" + r.ModuleID + "\x00" + r.Revision + "\x00" + r.Subdirectory
}

// Key identifies the full verified identity, digest included.
func (r SourceRef) Key() string { return r.Pin() + "\x00" + r.ContentSHA256 }

// Map returns the Driver Inputs form of the pin.
func (r SourceRef) Map() map[string]any {
	return map[string]any{
		"source_id": r.SourceID, "module_id": r.ModuleID, "revision": r.Revision,
		"subdirectory": r.Subdirectory, "content_sha256": r.ContentSHA256,
	}
}

// ParseSourceRef reads a submitted source object. It must hold exactly the
// five pin fields, each a string; no URL, path or credential is accepted.
func ParseSourceRef(value any) (SourceRef, error) {
	object, ok := value.(map[string]any)
	if !ok || object == nil {
		return SourceRef{}, errors.New("source must be an object")
	}
	for key := range object {
		if !contains(sourceRefFields, key) {
			return SourceRef{}, fmt.Errorf("source.%s is not supported; the source holds exactly %s", safeKey(key), strings.Join(sourceRefFields, ", "))
		}
	}
	text := map[string]string{}
	for _, field := range sourceRefFields {
		raw, present := object[field]
		value, isString := raw.(string)
		if !present || !isString {
			return SourceRef{}, fmt.Errorf("source.%s must be a string", field)
		}
		text[field] = value
	}
	ref := SourceRef{SourceID: text["source_id"], ModuleID: text["module_id"], Revision: text["revision"], Subdirectory: text["subdirectory"], ContentSHA256: text["content_sha256"]}
	return ref, ref.Validate()
}

// Input is one module variable the inspector found.
type Input struct {
	Type       string `json:"type"`
	Required   bool   `json:"required"`
	HasDefault bool   `json:"hasDefault"`
	Default    any    `json:"default,omitempty"`
	Sensitive  bool   `json:"sensitive"`
}

// Output is one module output. Type is empty when it cannot be derived
// statically; such an output is type-checked against the Resource Type after
// apply, before any consumer uses it.
type Output struct {
	Type      string `json:"type,omitempty"`
	Sensitive bool   `json:"sensitive"`
}

// ProviderRequirement is one provider a module requires, pinned to the exact
// release version and lock hashes.
type ProviderRequirement struct {
	Source      string   `json:"source"`
	Version     string   `json:"version"`
	Constraints string   `json:"constraints,omitempty"`
	Binding     string   `json:"binding"`
	Hashes      []string `json:"hashes"`
}

// TransitiveModule is one module directory the root module calls, directly
// or through other calls: a local directory of a pinned source revision or a
// remote child at an Admin-allowlisted Git source and exact full commit. Each
// is pinned by the digest of its directory content and cached with the
// bundle; nothing is downloaded at execution.
type TransitiveModule struct {
	SourceID      string `json:"sourceId"`
	Revision      string `json:"revision"`
	Path          string `json:"path"`
	ContentSHA256 string `json:"contentSha256"`
}

// ModuleCall is one module block instance of the call tree. Key is the
// Terraform module key ("net", "net.subnets"), Source the literal source of
// the block (verified content) and Transitive the index of the called
// directory in Contract.Transitive.
type ModuleCall struct {
	Key        string `json:"key"`
	Source     string `json:"source"`
	Transitive int    `json:"transitive"`
}

// Contract is the inspected, server-written bundle contract.
type Contract struct {
	SchemaVersion         int                `json:"schemaVersion"`
	SourceKind            string             `json:"sourceKind"`
	SourceURL             string             `json:"sourceUrl,omitempty"`
	SourceOrganizationKey string             `json:"sourceOrganizationKey,omitempty"`
	ResourceTypes         []string           `json:"resourceTypes"`
	RequiredVersion       string             `json:"requiredVersion,omitempty"`
	Inputs                map[string]Input   `json:"inputs"`
	Outputs               map[string]Output  `json:"outputs"`
	ExecutorOwned         []string           `json:"executorOwned"`
	ExecutorOutputs       []string           `json:"executorOutputs"`
	Transitive            []TransitiveModule `json:"transitive"`
	ModuleCalls           []ModuleCall       `json:"moduleCalls"`
}

// ContractSchemaVersion is the current Contract layout.
const ContractSchemaVersion = 1

// Bundle is one verified registry entry.
type Bundle struct {
	ID                    string                         `json:"id"`
	Ref                   SourceRef                      `json:"source"`
	TerraformVersion      string                         `json:"terraformVersion"`
	TerraformBinarySHA256 string                         `json:"terraformBinarySha256"`
	ProviderLockDigest    string                         `json:"providerLockDigest"`
	Contract              Contract                       `json:"contract"`
	Providers             map[string]ProviderRequirement `json:"providerRequirements"`
	Status                string                         `json:"status"`
	CreatedAt             time.Time                      `json:"createdAt"`
}

// Validate checks the stored shape of a bundle before it is persisted or
// trusted after a read.
func (b Bundle) Validate() error {
	if err := b.Ref.Validate(); err != nil {
		return err
	}
	switch {
	case !ValidVersion(b.TerraformVersion):
		return errors.New("bundle terraform version must be exact")
	case !ValidDigest(b.TerraformBinarySHA256) || !ValidDigest(b.ProviderLockDigest):
		return errors.New("bundle release digests must be SHA-256")
	case b.Status != StatusVerified:
		return errors.New("bundle status must be VERIFIED")
	case b.Contract.SchemaVersion != ContractSchemaVersion:
		return errors.New("bundle contract schema is not current")
	case b.Contract.SourceKind != SourceKindPackaged && b.Contract.SourceKind != SourceKindGit && b.Contract.SourceKind != SourceKindZIP:
		return errors.New("bundle source kind is unknown")
	case b.Contract.SourceKind == SourceKindZIP && b.Contract.SourceOrganizationKey == "":
		return errors.New("ZIP bundle organization is required")
	case b.Contract.Inputs == nil || b.Contract.Outputs == nil || b.Providers == nil:
		return errors.New("bundle contract is incomplete")
	}
	for _, t := range b.Contract.Transitive {
		if !ValidIdentifier(t.SourceID) || !revisionPattern.MatchString(t.Revision) || !ValidSubdirectory(t.Path) || !ValidDigest(t.ContentSHA256) {
			return errors.New("bundle transitive module pin is invalid")
		}
	}
	for _, call := range b.Contract.ModuleCalls {
		if call.Key == "" || call.Source == "" || call.Transitive < 0 || call.Transitive >= len(b.Contract.Transitive) {
			return errors.New("bundle module call is invalid")
		}
	}
	for name, provider := range b.Providers {
		if name == "" || provider.Source == "" || provider.Version == "" || len(provider.Hashes) == 0 {
			return errors.New("bundle provider requirement is incomplete")
		}
		switch provider.Binding {
		case BindingAWS, BindingKubernetes, BindingNone:
		default:
			return errors.New("bundle provider binding kind is unknown")
		}
	}
	return nil
}

// SameVerifiedContent reports whether two bundles for one pin describe the
// same verified content, contract and release. Any difference is drift.
func (b Bundle) SameVerifiedContent(other Bundle) bool {
	return b.Ref == other.Ref && b.TerraformVersion == other.TerraformVersion &&
		b.TerraformBinarySHA256 == other.TerraformBinarySHA256 &&
		b.ProviderLockDigest == other.ProviderLockDigest &&
		reflect.DeepEqual(normalize(b.Contract), normalize(other.Contract)) &&
		reflect.DeepEqual(b.Providers, other.Providers) && b.Status == other.Status
}

// normalize makes nil and empty slices compare equal after a JSON round trip.
func normalize(c Contract) Contract {
	if c.ResourceTypes == nil {
		c.ResourceTypes = []string{}
	}
	if c.ExecutorOwned == nil {
		c.ExecutorOwned = []string{}
	}
	if c.ExecutorOutputs == nil {
		c.ExecutorOutputs = []string{}
	}
	if c.Transitive == nil {
		c.Transitive = []TransitiveModule{}
	}
	if c.ModuleCalls == nil {
		c.ModuleCalls = []ModuleCall{}
	}
	inputs := map[string]Input{}
	for name, input := range c.Inputs {
		input.Default = jsonNumber(input.Default)
		inputs[name] = input
	}
	c.Inputs = inputs
	return c
}

// jsonNumber turns integer defaults into float64, the JSON decoding form.
func jsonNumber(v any) any {
	switch t := v.(type) {
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = jsonNumber(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, item := range t {
			out[k] = jsonNumber(item)
		}
		return out
	}
	return v
}

// ServesType reports whether the Admin/release policy pairs this bundle with
// the Resource Type. The pairing is never inferred from variable names.
func (b Bundle) ServesType(resourceType string) bool {
	return contains(b.Contract.ResourceTypes, resourceType)
}

// OutputNames lists module outputs plus executor-added outputs, sorted.
func (b Bundle) OutputNames() []string {
	names := make([]string, 0, len(b.Contract.Outputs)+len(b.Contract.ExecutorOutputs))
	for name := range b.Contract.Outputs {
		names = append(names, name)
	}
	names = append(names, b.Contract.ExecutorOutputs...)
	sort.Strings(names)
	return names
}

// EffectiveSecret is the monotonic secret classification of one mapped
// output: secret when either the Resource Type or the module marks it.
// Mapping never downgrades a secret to a displayable value.
func EffectiveSecret(typeSecret, moduleSensitive bool) bool { return typeSecret || moduleSensitive }

// Binding is one provider binding of a Definition.
type Binding struct {
	Target      string `json:"target"`
	ResourceRef string `json:"resource_ref,omitempty"`
}

// DriverInputs is the strict Terraform `driverInputs.values` shape.
type DriverInputs struct {
	Source           SourceRef
	Variables        map[string]any
	InputMapping     map[string]string // module variable -> Resource Type input
	OutputMapping    map[string]string // module output -> Resource Type output
	ProviderBindings map[string]Binding
}

var driverValueFields = []string{"source", "variables", "input_mapping", "output_mapping", "provider_bindings"}

// ParseDriverInputs reads `driverInputs.values` of a Terraform Definition.
// Unknown or null fields, arbitrary expressions and secret fields are
// rejected; errors name the field path, never a submitted value.
func ParseDriverInputs(values map[string]any) (DriverInputs, error) {
	if values == nil {
		return DriverInputs{}, errors.New("driverInputs.values must be an object")
	}
	for key := range values {
		if !contains(driverValueFields, key) {
			return DriverInputs{}, fmt.Errorf("driverInputs.values.%s is not a supported field", safeKey(key))
		}
	}
	out := DriverInputs{InputMapping: map[string]string{}, OutputMapping: map[string]string{}, ProviderBindings: map[string]Binding{}}
	source, err := ParseSourceRef(values["source"])
	if err != nil {
		return DriverInputs{}, fmt.Errorf("driverInputs.values.%w", err)
	}
	out.Source = source
	variables, ok := values["variables"].(map[string]any)
	if !ok || variables == nil {
		return DriverInputs{}, errors.New("driverInputs.values.variables must be an object")
	}
	out.Variables = variables
	if raw, present := values["input_mapping"]; present {
		object, ok := raw.(map[string]any)
		if !ok || object == nil {
			return DriverInputs{}, errors.New("driverInputs.values.input_mapping must be an object")
		}
		for name, entry := range object {
			mapping, ok := entry.(map[string]any)
			param, isString := mapping["param"].(string)
			if !ok || len(mapping) != 1 || !isString || param == "" {
				return DriverInputs{}, fmt.Errorf("driverInputs.values.input_mapping.%s must be {param: <Resource Type input>}", safeKey(name))
			}
			out.InputMapping[name] = param
		}
	}
	if raw, present := values["output_mapping"]; present {
		object, ok := raw.(map[string]any)
		if !ok || object == nil {
			return DriverInputs{}, errors.New("driverInputs.values.output_mapping must be an object")
		}
		for name, entry := range object {
			target, ok := entry.(string)
			if !ok || target == "" {
				return DriverInputs{}, fmt.Errorf("driverInputs.values.output_mapping.%s must name a Resource Type output", safeKey(name))
			}
			out.OutputMapping[name] = target
		}
	}
	raw, present := values["provider_bindings"]
	if !present {
		return DriverInputs{}, errors.New("driverInputs.values.provider_bindings must be an object")
	}
	bindings, ok := raw.(map[string]any)
	if !ok || bindings == nil {
		return DriverInputs{}, errors.New("driverInputs.values.provider_bindings must be an object")
	}
	for name, entry := range bindings {
		object, ok := entry.(map[string]any)
		if !ok || object == nil {
			return DriverInputs{}, fmt.Errorf("driverInputs.values.provider_bindings.%s must be an object", safeKey(name))
		}
		binding := Binding{}
		for key, value := range object {
			text, isString := value.(string)
			if !isString {
				return DriverInputs{}, fmt.Errorf("driverInputs.values.provider_bindings.%s.%s must be a string", safeKey(name), safeKey(key))
			}
			switch key {
			case "target":
				binding.Target = text
			case "resource_ref":
				binding.ResourceRef = text
			default:
				// No account, region, endpoint or credential override.
				return DriverInputs{}, fmt.Errorf("driverInputs.values.provider_bindings.%s.%s is not supported", safeKey(name), safeKey(key))
			}
		}
		switch {
		case binding.Target == TargetAccount && binding.ResourceRef == "":
		case binding.Target == TargetResource && binding.ResourceRef != "":
		default:
			return DriverInputs{}, fmt.Errorf("driverInputs.values.provider_bindings.%s must be {target: account} or {target: resource, resource_ref: <descriptor>}", safeKey(name))
		}
		out.ProviderBindings[name] = binding
	}
	return out, nil
}

// safeKey bounds a caller-supplied object key in an error message.
func safeKey(key string) string {
	if len(key) > 64 {
		key = key[:64]
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '?'
		}
		return r
	}, key)
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// ModuleInstall locates one verified module call in the private cache, for
// the executor to install without any download.
type ModuleInstall struct {
	Key    string
	Source string
	Dir    string
}

// VerifiedArtifact is a bundle whose cached content, release pins and
// Terraform binary were verified immediately before execution and copied into
// one private, invocation-owned directory (Root). Every path below points into
// that copy, never into the shared cache or the configured binary location, so
// nothing verified can be replaced between verification and execution:
// SourceDir is the materialized source root, ModuleDir the root module inside
// it, LockFile the readonly provider lock the release pins, TerraformBinary
// the private executable copy whose digest was checked while it was written
// (the executor must run exactly that path) and Modules the verified module
// call tree. Close removes the private copy.
type VerifiedArtifact struct {
	Bundle          Bundle
	Root            string
	SourceDir       string
	ModuleDir       string
	LockFile        []byte
	TerraformBinary string
	Modules         []ModuleInstall
	// Cleanup removes the private invocation copy; use Close.
	Cleanup func() error
	// Retain records a private handle -> workspace mapping so an operator can
	// locate this invocation directory after the executor kept it (state or
	// credentials that may be the only copy, or a process that may still use
	// it). The mapping is internal; the handle alone reveals no path.
	Retain func(handle string) error
}

// Close removes the private invocation copy once. It is safe on a nil
// receiver or an artifact without a private copy.
func (a *VerifiedArtifact) Close() error {
	if a == nil || a.Cleanup == nil {
		return nil
	}
	cleanup := a.Cleanup
	a.Cleanup = nil
	return cleanup()
}

// Clone returns a deep copy, so no holder of a verified bundle can change
// another holder's contract, provider hashes or defaults in place.
func (b Bundle) Clone() Bundle {
	out := b
	out.Contract = b.Contract.clone()
	if b.Providers != nil {
		out.Providers = make(map[string]ProviderRequirement, len(b.Providers))
		for name, p := range b.Providers {
			p.Hashes = append([]string(nil), p.Hashes...)
			out.Providers[name] = p
		}
	}
	return out
}

func (c Contract) clone() Contract {
	out := c
	out.ResourceTypes = cloneStrings(c.ResourceTypes)
	out.ExecutorOwned = cloneStrings(c.ExecutorOwned)
	out.ExecutorOutputs = cloneStrings(c.ExecutorOutputs)
	if c.Transitive != nil {
		out.Transitive = append([]TransitiveModule(nil), c.Transitive...)
	}
	if c.ModuleCalls != nil {
		out.ModuleCalls = append([]ModuleCall(nil), c.ModuleCalls...)
	}
	if c.Inputs != nil {
		out.Inputs = make(map[string]Input, len(c.Inputs))
		for name, input := range c.Inputs {
			input.Default = cloneValue(input.Default)
			out.Inputs[name] = input
		}
	}
	if c.Outputs != nil {
		out.Outputs = make(map[string]Output, len(c.Outputs))
		for name, output := range c.Outputs {
			out.Outputs[name] = output
		}
	}
	return out
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	return append([]string(nil), in...)
}

func cloneValue(v any) any {
	switch t := v.(type) {
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = cloneValue(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, item := range t {
			out[k] = cloneValue(item)
		}
		return out
	}
	return v
}

// ProviderPin is the plan-time resolved, nonsecret metadata of one provider
// binding of a Terraform node: which release provider it configures, whether
// the Definition Driver Account or the referenced cluster resource supplies the target,
// and the scoped identity that target was resolved to. It carries no
// credential, endpoint or account override; execution resolves the credential
// again from this identity and refuses any difference.
type ProviderPin struct {
	// Name is the provider requirement name of the module (aws, kubernetes).
	Name string `json:"name"`
	// Kind is the release binding kind (BindingAWS or BindingKubernetes).
	Kind string `json:"kind"`
	// Target is TargetAccount or TargetResource, as declared.
	Target string `json:"target"`
	// ConnectionKey and ConnectionKind name the Connection that supplies
	// credentials: the Definition's Driver Account for an AWS binding, the
	// Environment connection for a legacy Kubernetes binding.
	ConnectionKey  string `json:"connectionKey,omitempty"`
	ConnectionKind string `json:"connectionKind,omitempty"`
	// AccountVersion pins the Driver Account version the plan saw.
	AccountVersion int64 `json:"accountVersion,omitempty"`
	// Region is the literal Driver Inputs region of an AWS binding.
	Region string `json:"region,omitempty"`
	// Provider is the descriptor of the cluster resource node that supplies
	// the target of a resource binding (the dependency edge points at it).
	Provider string `json:"provider,omitempty"`
	// ClusterProvider and ClusterAccount pin a Kubernetes binding whose cluster is created by a
	// Terraform EKS module in the same plan (ADR-017): the producing cluster's AWS Driver Account
	// (AccountVersion and Region describe it). No Kubernetes Connection supplies credentials then;
	// execution derives fresh short-lived authentication from that original account.
	ClusterProvider string `json:"clusterProvider,omitempty"`
	ClusterAccount  string `json:"clusterAccount,omitempty"`
	// Generation pins the identity generation of the Environment's resources.
	Generation int64 `json:"generation,omitempty"`
}
