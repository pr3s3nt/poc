package accountdiag

import (
	"sort"

	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/diagnostics"
	probes "orchestrator/internal/ports/diagnostics"
)

// Resource types a selected capability stands for. The requirements below come
// from the matched Definitions and the deployed configuration, never from a
// module or Definition name.
var capabilityTypes = map[string]map[string]bool{
	"database": {"postgres": true, "mysql": true, "mariadb": true, "mongodb": true, "database": true},
	"cache":    {"redis": true, "memcached": true, "cache": true},
}

// platformChecks are the checks the service evaluates itself from the
// resolved Application/Environment requirements and earlier provider findings.
func platformChecks(scope domain.Scope, provider []probes.Check) []probes.Check {
	if scope.Purpose != domain.PurposeUseCase {
		return nil
	}
	has := map[string]bool{}
	for _, c := range provider {
		has[c.ID] = true
	}
	out := []probes.Check{
		{ID: "uc.context", Group: "usecase", AffectedUseCases: scope.UseCases},
		{ID: "uc.definitions", Group: "usecase", DependsOn: []string{"uc.context"}, AffectedUseCases: scope.Affects("preview", "deploy", "database", "cache")},
	}
	for _, use := range scope.UseCases {
		deps := []string{"uc.context"}
		switch use {
		case "database", "cache":
			deps = []string{"uc.definitions", "k8s.storage.classes", "k8s.storage.provisioner"}
		case "deploy":
			deps = []string{"uc.context", "k8s.nodes"}
		case "lifecycle":
			continue
		}
		out = append(out, probes.Check{ID: "uc." + use + ".requirements", Group: "usecase", DependsOn: deps, AffectedUseCases: []string{use}})
	}
	return out
}

// contextGap says why Application-specific facts are not known, or "".
func contextGap(need *probes.Requirements) string {
	switch {
	case need == nil:
		return "APPLICATION_CONTEXT_REQUIRED"
	case need.EnvironmentKey == "":
		return "ENVIRONMENT_CONTEXT_REQUIRED"
	case !need.Resolved:
		return "NO_DEPLOYED_CONFIGURATION"
	}
	return ""
}

func unknownContext(reason string) domain.Finding {
	return domain.Finding{Verdict: domain.VerdictUnknown, Reason: reason, Remediation: "select-context", Facts: map[string]string{"contextProvided": "false"}}
}

// evalPlatform evaluates one uc.* check. It reads the desired state only
// through the resolved requirements and never contacts the account.
func evalPlatform(c probes.Check, scope domain.Scope, acct application.Connection, need *probes.Requirements, prior map[string]domain.Finding) domain.Finding {
	gap := contextGap(need)
	pass := func(reason string, facts map[string]string) domain.Finding {
		return domain.Finding{Verdict: domain.VerdictPass, Reason: reason, Facts: facts}
	}
	na := func(reason string) domain.Finding { return domain.Finding{Verdict: domain.VerdictNA, Reason: reason} }
	switch c.ID {
	case "uc.context":
		if gap != "" {
			return unknownContext(gap)
		}
		return pass("CONTEXT_RESOLVED", map[string]string{"workloads": domain.Itoa(need.Workloads), "resources": domain.Itoa(len(need.Resources)), "publicRoutes": domain.Itoa(need.PublicRoutes), "contextProvided": "true"})
	case "uc.definitions":
		if gap != "" {
			return unknownContext(gap)
		}
		if len(need.Resources) == 0 {
			return na("NO_RESOURCES")
		}
		var unmatched, ambiguous int
		types := map[string]bool{}
		keys := map[string]bool{}
		for _, r := range need.Resources {
			switch r.Outcome {
			case "NO_MATCH":
				unmatched++
				types[r.Type] = true
			case "AMBIGUOUS":
				ambiguous++
				types[r.Type] = true
			default:
				keys[r.Definition] = true
			}
		}
		switch {
		case ambiguous > 0:
			return domain.Finding{Verdict: domain.VerdictFail, Reason: "DEFINITION_AMBIGUOUS", Remediation: "narrow-definition", Facts: map[string]string{"unmatched": domain.Itoa(unmatched + ambiguous), "resourceTypes": domain.JoinList(sortedKeys(types))}}
		case unmatched > 0:
			return domain.Finding{Verdict: domain.VerdictFail, Reason: "DEFINITION_NOT_MATCHED", Remediation: "register-definition", Facts: map[string]string{"unmatched": domain.Itoa(unmatched), "resourceTypes": domain.JoinList(sortedKeys(types))}}
		}
		return pass("ALL_RESOURCES_MATCHED", map[string]string{"matches": domain.Itoa(len(need.Resources)), "definitionKeys": domain.JoinList(sortedKeys(keys))})
	case "uc.preview.requirements":
		if gap != "" {
			return unknownContext(gap)
		}
		return pass("INPUTS_RESOLVED", map[string]string{"workloads": domain.Itoa(need.Workloads)})
	case "uc.deploy.requirements":
		if gap != "" {
			return unknownContext(gap)
		}
		if need.Workloads == 0 {
			return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "NO_WORKLOADS", Remediation: "add-workload"}
		}
		return pass("WORKLOADS_DECLARED", map[string]string{"workloads": domain.Itoa(need.Workloads)})
	case "uc.secrets.requirements":
		if gap != "" {
			return unknownContext(gap)
		}
		switch {
		case need.SecretStoreKey == "":
			return domain.Finding{Verdict: domain.VerdictFail, Reason: "NO_SECRET_STORE_SELECTED", Remediation: "select-secret-store"}
		case !need.SecretStoreReady:
			return domain.Finding{Verdict: domain.VerdictFail, Reason: "SECRET_STORE_NOT_READY", Remediation: "verify-secret-store", Facts: map[string]string{"secretStore": need.SecretStoreKey}}
		}
		return pass("SECRET_STORE_READY", map[string]string{"secretStore": need.SecretStoreKey})
	case "uc.https.requirements":
		if gap != "" {
			return unknownContext(gap)
		}
		if need.PublicRoutes == 0 {
			return na("NO_PUBLIC_ROUTES")
		}
		return pass("ROUTES_DECLARED", map[string]string{"publicRoutes": domain.Itoa(need.PublicRoutes)})
	case "uc.database.requirements", "uc.cache.requirements":
		use := c.AffectedUseCases[0]
		if gap != "" {
			return unknownContext(gap)
		}
		return evalCapability(use, scope, acct, need, prior)
	}
	return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "PROBE_ERROR"}
}

func evalCapability(use string, scope domain.Scope, acct application.Connection, need *probes.Requirements, prior map[string]domain.Finding) domain.Finding {
	var relevant, placed, storage []probes.ResourceNeed
	for _, r := range need.Resources {
		if !capabilityTypes[use][r.Type] {
			continue
		}
		relevant = append(relevant, r)
		if r.DriverAccount == acct.Key || (r.DriverAccount == "" && scope.AccountType == "INTERNAL") {
			placed = append(placed, r)
			if r.NeedsStorage {
				storage = append(storage, r)
			}
		}
	}
	facts := map[string]string{"relevant": domain.Itoa(len(relevant)), "contextProvided": "true"}
	switch {
	case len(relevant) == 0:
		return domain.Finding{Verdict: domain.VerdictNA, Reason: "NO_RESOURCE_FOR_USE_CASE"}
	case len(placed) == 0:
		return domain.Finding{Verdict: domain.VerdictNA, Reason: "PLACED_ON_OTHER_ACCOUNT", Facts: facts}
	case scope.AccountType == "AWS":
		facts["resources"] = domain.Itoa(len(placed))
		return domain.Finding{Verdict: domain.VerdictPass, Reason: "PLACED_ON_ACCOUNT", Facts: facts}
	case len(storage) == 0:
		return domain.Finding{Verdict: domain.VerdictNA, Reason: "STORAGE_NOT_REQUIRED", Facts: facts}
	}
	classes := prior["k8s.storage.classes"]
	if classes.Verdict != domain.VerdictPass {
		return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "STORAGE_EVIDENCE_UNAVAILABLE", Facts: facts}
	}
	have := map[string]bool{}
	for _, name := range splitList(classes.Facts["classes"]) {
		have[name] = true
	}
	missing, needDefault := map[string]bool{}, false
	for _, r := range storage {
		switch {
		case r.StorageClass == "":
			needDefault = true
		case !have[r.StorageClass]:
			missing[r.StorageClass] = true
		}
	}
	facts["storageNeeds"] = domain.Itoa(len(storage))
	switch {
	case len(missing) > 0:
		facts["requiredClasses"] = domain.JoinList(sortedKeys(missing))
		return domain.Finding{Verdict: domain.VerdictFail, Reason: "STORAGE_CLASS_MISSING", Remediation: "create-storage-class", Facts: facts}
	case needDefault && classes.Facts["defaultClasses"] == "0":
		return domain.Finding{Verdict: domain.VerdictFail, Reason: "NO_DEFAULT_STORAGE_CLASS", Remediation: "set-default-storage-class", Facts: facts}
	}
	return domain.Finding{Verdict: domain.VerdictPass, Reason: "STORAGE_REQUIREMENTS_MET", Facts: facts}
}

func splitList(v string) []string {
	var out []string
	cur := ""
	for _, r := range v {
		if r == ',' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	sort.Strings(out)
	return out
}
