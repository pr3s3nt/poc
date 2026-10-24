package diagnostics

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// factKind validates one registered fact key. A fact is never free text: the
// key must be registered and the value must have exactly the key's shape, so
// arbitrary provider strings cannot enter a report.
type factKind func(string) bool

func re(expr string) factKind {
	pattern := regexp.MustCompile(expr)
	return func(v string) bool { return len(v) <= 256 && pattern.MatchString(v) }
}

func listOf(item factKind, max int) factKind {
	return func(v string) bool {
		parts := strings.Split(v, ",")
		if len(v) > 1024 || len(parts) > max {
			return false
		}
		for _, p := range parts {
			if !item(p) {
				return false
			}
		}
		return true
	}
}

const (
	nameExpr = `[A-Za-z0-9]([A-Za-z0-9._/-]{0,126}[A-Za-z0-9])?`
)

var (
	kCount    = re(`^[0-9]{1,9}$`)
	kInt      = re(`^-?[0-9]{1,6}$`)
	kVersion  = re(`^v?[0-9]+(\.[0-9]+){0,3}([-+][0-9A-Za-z.-]{1,40})?$`)
	kName     = re(`^` + nameExpr + `$`)
	kList     = listOf(kName, 16)
	kVerList  = listOf(kVersion, 8)
	kBool     = re(`^(true|false)$`)
	kEnum     = re(`^[A-Z][A-Z0-9_]{0,40}$`)
	kAccount  = re(`^[0-9]{12}$`)
	kRegion   = re(`^[a-z]{2}(-[a-z]+)+-[0-9]$`)
	kHostPort = re(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?(:[0-9]{1,5})?$`)
	kARN      = re(`^arn:aws:(iam|sts)::[0-9]{12}:[A-Za-z0-9+=,.@_/-]{1,256}$`)
	kUID      = re(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	kTime     = func(v string) bool { _, err := time.Parse(time.RFC3339, v); return err == nil }
)

// factKinds is the complete registry of fact keys that probes may report.
var factKinds = map[string]factKind{
	// counts and measures
	"nodes": kCount, "nodesReady": kCount, "storageClasses": kCount, "defaultClasses": kCount,
	"csiDrivers": kCount, "csiNodes": kCount, "found": kCount, "denied": kCount, "allowed": kCount,
	"servicesChecked": kCount, "publicRoutes": kCount, "resources": kCount, "unmatched": kCount, "relevant": kCount, "storageNeeds": kCount, "groupsChecked": kCount, "groupsAllowed": kCount, "actions": kCount,
	"replicasReady": kCount, "definitions": kCount, "matches": kCount, "workloads": kCount,
	"daysRemaining": kInt, "latencyMs": kCount,
	// versions
	"serverVersion": kVersion, "kubeletVersions": kVerList,
	// identifiers
	"namespace": kName, "storageClass": kName, "provisioner": kName, "driver": kName, "application": kName,
	"environment": kName, "definition": kName, "resourceType": kName, "secretStore": kName, "module": kName,
	"controller": kName, "workload": kName, "podPhase": kName, "claimPhase": kName, "cluster": kName,
	"service": kName, "group": kName, "capability": kName,
	// lists
	"deniedPermissions": kList, "missingResources": kList, "classes": kList, "drivers": kList,
	"deniedGroups": kList, "unprovenGroups": kList, "missingCrds": kList, "regions": kList, "services": kList,
	"installed": kList, "ingressClasses": kList, "requiredClasses": kList, "definitionKeys": kList, "resourceTypes": kList,
	// flags and enums
	"insecure": kBool, "sealed": kBool, "tokenAccepted": kBool, "pending": kCount, "stage": kName, "planReason": kName, "providers": kList, "classesTested": kList, "reclaimPolicy": kEnum, "default": kBool, "hostMatch": kBool, "required": kBool, "contextProvided": kBool,
	"tlsMode": kEnum, "principalType": kEnum, "evidence": kEnum, "role": kEnum, "groups": kList,
	// remote identity
	"host": kHostPort, "endpoint": kHostPort, "accountId": kAccount, "region": kRegion,
	"principalArn": kARN, "roleArn": kARN, "uid": kUID,
	// times
	"notAfter": kTime, "notBefore": kTime,
}

// identityKinds are the safe account target fields a Scope may pin.
var identityKinds = map[string]factKind{
	"cluster": kName, "endpoint": kHostPort, "accountId": kAccount, "roleArn": kARN, "principalType": kEnum, "authentication": kEnum,
}

// ValidFact reports whether key/value is an allowed fact.
func ValidFact(key, value string) bool {
	kind, ok := factKinds[key]
	return ok && kind(value)
}

// Sanitized returns f keeping only registered, well-shaped facts; everything
// else is dropped, never partially kept. Adapters call it before returning a
// finding, and repositories reject a finding that still carries a bad fact.
func (f Finding) Sanitized() Finding {
	if len(f.Facts) > 0 {
		clean := map[string]string{}
		for k, v := range f.Facts {
			if ValidFact(k, v) && len(clean) < MaxFacts {
				clean[k] = v
			}
		}
		f.Facts = nil
		if len(clean) > 0 {
			f.Facts = clean
		}
	}
	return f.Clone()
}

// Clone deep-copies f.
func (f Finding) Clone() Finding {
	if f.Facts != nil {
		facts := make(map[string]string, len(f.Facts))
		for k, v := range f.Facts {
			facts[k] = v
		}
		f.Facts = facts
	}
	f.AffectedUseCases = append([]string(nil), f.AffectedUseCases...)
	f.DependsOn = append([]string(nil), f.DependsOn...)
	if len(f.AffectedUseCases) == 0 {
		f.AffectedUseCases = nil
	}
	if len(f.DependsOn) == 0 {
		f.DependsOn = nil
	}
	return f
}

// Validate checks the shape of a stored finding, including every fact.
func (f Finding) Validate() error {
	if !codePattern.MatchString(f.ID) || !codePattern.MatchString(f.Code) || !f.Verdict.Valid() || !reasonPattern.MatchString(f.Reason) {
		return ErrInvalidReport
	}
	if f.Group == "" || len(f.Group) > 32 || len(f.Facts) > MaxFacts || len(f.DependsOn) > MaxDependsOn {
		return ErrInvalidReport
	}
	if f.Remediation != "" && !codePattern.MatchString(f.Remediation) {
		return ErrInvalidReport
	}
	for k, v := range f.Facts {
		if !ValidFact(k, v) {
			return fmt.Errorf("%w: fact %s", ErrInvalidReport, k)
		}
	}
	for _, u := range f.AffectedUseCases {
		if !supported(u) {
			return ErrInvalidReport
		}
	}
	for _, d := range f.DependsOn {
		if !codePattern.MatchString(d) || d == f.ID {
			return ErrInvalidReport
		}
	}
	return nil
}

func validIdentity(m map[string]string) bool {
	if len(m) > 8 {
		return false
	}
	for k, v := range m {
		kind, ok := identityKinds[k]
		if !ok || !kind(v) {
			return false
		}
	}
	return true
}

// ProbeResource states.
const (
	ResourceIntended  = "INTENDED"  // recorded before the create call; UID unknown
	ResourceCreated   = "CREATED"   // acknowledged with an observed UID
	ResourceDeleted   = "DELETED"   // deleted by this run and absence observed
	ResourceAbsent    = "ABSENT"    // intended but observed absent
	ResourcePreserved = "PRESERVED" // a foreign or replaced object was found and left alone
	ResourceUnknown   = "UNKNOWN"   // deletion or presence could not be confirmed
)

var resourceKinds = map[string]bool{"namespace": true, "persistentvolumeclaim": true, "persistentvolume": true, "pod": true}

// Validate checks one run-owned resource record.
func (p ProbeResource) Validate() error {
	switch p.State {
	case ResourceIntended, ResourceCreated, ResourceDeleted, ResourceAbsent, ResourcePreserved, ResourceUnknown:
	default:
		return ErrInvalidReport
	}
	if !resourceKinds[p.Kind] || !dnsLabel.MatchString(p.Name) || (p.Namespace != "" && !dnsLabel.MatchString(p.Namespace)) {
		return ErrInvalidReport
	}
	if p.UID != "" && !kUID(p.UID) {
		return ErrInvalidReport
	}
	if (p.State == ResourceCreated || p.State == ResourceDeleted) && p.UID == "" {
		return ErrInvalidReport
	}
	return nil
}

// Settled reports whether the resource needs no further cleanup.
func (p ProbeResource) Settled() bool {
	return p.State == ResourceDeleted || p.State == ResourceAbsent || p.State == ResourcePreserved
}

var dnsLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

func resourceKey(p ProbeResource) string { return p.Kind + "/" + p.Namespace + "/" + p.Name }

// AllSettled reports whether every recorded resource is settled.
func AllSettled(resources []ProbeResource) bool {
	for _, p := range resources {
		if !p.Settled() {
			return false
		}
	}
	return true
}

// Itoa is a small helper for probes building count facts.
func Itoa(n int) string { return strconv.Itoa(n) }

// JoinList builds a list fact from at most 16 names, sorted by the caller.
func JoinList(items []string) string {
	if len(items) > 16 {
		items = items[:16]
	}
	return strings.Join(items, ",")
}

// ValidIdentity reports whether key/value is an allowed scope identity field.
func ValidIdentity(key, value string) bool {
	kind, ok := identityKinds[key]
	return ok && kind(value)
}

var capabilityTypes = map[string]string{
	"postgres": "database", "mysql": "database", "mariadb": "database", "mongodb": "database", "database": "database",
	"redis": "cache", "memcached": "cache", "cache": "cache",
}

// CapabilityOf says which selectable capability ("database" or "cache") a
// resource type belongs to, or "" when it belongs to none of them.
func CapabilityOf(resourceType string) string { return capabilityTypes[resourceType] }
