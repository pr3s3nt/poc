package accountdiag

import (
	"context"
	"sort"

	"orchestrator/internal/domain/application"
	domain "orchestrator/internal/domain/diagnostics"
	probes "orchestrator/internal/ports/diagnostics"
)

// platformChecks are the checks the service itself evaluates from the resolved
// requirements and earlier provider findings (some read real evidence through
// the injected inspectors).
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
		{ID: "uc.preview.requirements", Group: "usecase", DependsOn: []string{"uc.context"}, AffectedUseCases: scope.Affects("preview", "deploy", "database", "cache")},
	}
	for _, use := range scope.UseCases {
		switch use {
		case "deploy":
			out = append(out, probes.Check{ID: "uc.deploy.requirements", Group: "usecase", DependsOn: []string{"uc.preview.requirements", "k8s.nodes"}, AffectedUseCases: []string{use}})
		case "database", "cache":
			out = append(out, probes.Check{ID: "uc." + use + ".requirements", Group: "usecase", DependsOn: []string{"uc.preview.requirements", "k8s.storage.requirements", "k8s.storage.provisioner"}, AffectedUseCases: []string{use}})
		case "secrets":
			out = append(out,
				probes.Check{ID: "uc.secrets.store", Group: "usecase", DependsOn: []string{"uc.context"}, AffectedUseCases: []string{use}},
				probes.Check{ID: "uc.secrets.workload", Group: "usecase", DependsOn: []string{"uc.secrets.store"}, AffectedUseCases: []string{use}})
		case "https":
			out = append(out,
				probes.Check{ID: "uc.https.requirements", Group: "usecase", DependsOn: []string{"uc.context"}, AffectedUseCases: []string{use}},
				probes.Check{ID: "uc.https.certificate", Group: "usecase", DependsOn: []string{"uc.https.requirements", "k8s.route.controller"}, AffectedUseCases: []string{use}})
		}
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
	}
	return ""
}

func unknownContext(reason string) domain.Finding {
	return domain.Finding{Verdict: domain.VerdictUnknown, Reason: reason, Remediation: "select-context", Facts: map[string]string{"contextProvided": "false"}}
}

// planGap describes a planning result that cannot support a requirement.
func planGap(need *probes.Requirements) (domain.Finding, bool) {
	switch {
	case need.PlanUnavailable:
		return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "PLAN_UNAVAILABLE", Remediation: "rerun"}, true
	case !need.Resolved:
		facts := map[string]string{}
		if need.PlanStage != "" {
			facts["stage"] = need.PlanStage
		}
		if need.PlanReason != "" {
			facts["planReason"] = need.PlanReason
		}
		return domain.Finding{Verdict: domain.VerdictFail, Reason: "PLANNING_REJECTED", Remediation: "fix-desired-state", Facts: facts}, true
	}
	return domain.Finding{}, false
}

// evalPlatform evaluates one uc.* check.
func (s *Service) evalPlatform(ctx context.Context, c probes.Check, scope domain.Scope, acct application.Connection, need *probes.Requirements, org string, prior map[string]domain.Finding) domain.Finding {
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
		return pass("CONTEXT_RESOLVED", map[string]string{"workloads": domain.Itoa(need.Workloads), "resources": domain.Itoa(len(need.Resources)), "publicRoutes": domain.Itoa(need.PublicRoutes), "pending": domain.Itoa(need.PendingChanges), "contextProvided": "true"})
	case "uc.preview.requirements":
		if gap != "" {
			return unknownContext(gap)
		}
		if f, bad := planGap(need); bad {
			return f
		}
		if need.Workloads == 0 {
			return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "NO_WORKLOADS", Remediation: "add-workload"}
		}
		return pass("PLAN_RESOLVED", map[string]string{"workloads": domain.Itoa(need.Workloads), "resources": domain.Itoa(len(need.Resources)), "pending": domain.Itoa(need.PendingChanges)})
	case "uc.deploy.requirements":
		return deployRequirement(scope, acct, need, gap)
	case "uc.secrets.store":
		if gap != "" {
			return unknownContext(gap)
		}
		if need.SecretStoreKey == "" {
			return domain.Finding{Verdict: domain.VerdictFail, Reason: "NO_SECRET_STORE_SELECTED", Remediation: "select-secret-store"}
		}
		if s.stores == nil {
			return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "STORE_INSPECTOR_UNAVAILABLE", Facts: map[string]string{"secretStore": need.SecretStoreKey}}
		}
		f := s.stores.Inspect(ctx, org, need.SecretStoreKey)
		if f.Facts == nil {
			f.Facts = map[string]string{}
		}
		f.Facts["secretStore"] = need.SecretStoreKey
		return f
	case "uc.secrets.workload":
		if gap != "" {
			return unknownContext(gap)
		}
		// What workload pods can reach and whether their login role works is
		// decided at deploy time inside the cluster: the platform runtime cannot
		// observe it, so it is never reported as ready.
		return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "WORKLOAD_AUTH_UNOBSERVABLE", Facts: map[string]string{"evidence": "NONE"}}
	case "uc.https.requirements":
		if gap != "" {
			return unknownContext(gap)
		}
		if f, bad := planGap(need); bad && need.PublicRoutes == 0 {
			return f
		}
		if need.PublicRoutes == 0 {
			return na("NO_PUBLIC_ROUTES")
		}
		return pass("ROUTES_DECLARED", map[string]string{"publicRoutes": domain.Itoa(need.PublicRoutes)})
	case "uc.https.certificate":
		if gap != "" {
			return unknownContext(gap)
		}
		if need.PublicRoutes == 0 {
			return na("NO_PUBLIC_ROUTES")
		}
		if need.PublicHost == "" {
			return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "PUBLIC_HOST_UNKNOWN", Remediation: "set-subdomain"}
		}
		if s.routes == nil {
			return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "ROUTE_INSPECTOR_UNAVAILABLE"}
		}
		return s.routes.Inspect(ctx, need.PublicHost)
	case "uc.database.requirements", "uc.cache.requirements":
		if gap != "" {
			return unknownContext(gap)
		}
		if f, bad := planGap(need); bad {
			return f
		}
		return evalCapability(c.AffectedUseCases[0], scope, acct, need, prior)
	}
	return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "PROBE_ERROR"}
}

// placedHere reports whether a planned resource runs on this account.
func placedHere(r probes.ResourceNeed, scope domain.Scope, acct application.Connection) bool {
	for _, p := range r.Providers {
		if p.Connection == acct.Key && ((scope.AccountType == "AWS" && p.Kind == "aws") || (scope.AccountType == "INTERNAL" && p.Kind == "kubernetes")) {
			return true
		}
	}
	return scope.AccountType == "INTERNAL" && r.Cluster == acct.Key
}

func deployRequirement(scope domain.Scope, acct application.Connection, need *probes.Requirements, gap string) domain.Finding {
	if gap != "" {
		return unknownContext(gap)
	}
	if f, bad := planGap(need); bad {
		return f
	}
	if need.Workloads == 0 {
		return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "NO_WORKLOADS", Remediation: "add-workload"}
	}
	if scope.AccountType == "AWS" {
		return domain.Finding{Verdict: domain.VerdictNA, Reason: "ACCOUNT_NOT_USED_BY_DEPLOY"}
	}
	here, elsewhere := 0, 0
	for _, p := range need.Placements {
		if p.Account == acct.Key {
			here++
		} else if p.Account != "" {
			elsewhere++
		}
	}
	facts := map[string]string{"workloads": domain.Itoa(need.Workloads), "relevant": domain.Itoa(here)}
	switch {
	case here == 0 && elsewhere > 0:
		return domain.Finding{Verdict: domain.VerdictNA, Reason: "PLACED_ON_OTHER_ACCOUNT", Facts: facts}
	case here == 0:
		return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "PLACEMENT_UNDETERMINED", Facts: facts}
	}
	return domain.Finding{Verdict: domain.VerdictPass, Reason: "DEPLOY_TARGET_AGREES", Facts: facts}
}

func evalCapability(use string, scope domain.Scope, acct application.Connection, need *probes.Requirements, prior map[string]domain.Finding) domain.Finding {
	var relevant, placed []probes.ResourceNeed
	for _, r := range need.Resources {
		if domain.CapabilityOf(r.Type) != use {
			continue
		}
		relevant = append(relevant, r)
		if placedHere(r, scope, acct) {
			placed = append(placed, r)
		}
	}
	facts := map[string]string{"relevant": domain.Itoa(len(relevant)), "contextProvided": "true"}
	switch {
	case len(relevant) == 0:
		return domain.Finding{Verdict: domain.VerdictNA, Reason: "NO_RESOURCE_FOR_USE_CASE"}
	case len(placed) == 0:
		return domain.Finding{Verdict: domain.VerdictNA, Reason: "PLACED_ON_OTHER_ACCOUNT", Facts: facts}
	}
	facts["resources"] = domain.Itoa(len(placed))
	if scope.AccountType == "AWS" {
		for _, r := range placed {
			for _, p := range r.Providers {
				if p.Connection != acct.Key || p.Kind != "aws" {
					continue
				}
				if p.Region == "" {
					return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "REGION_UNPINNED", Facts: facts, Remediation: "pin-region"}
				}
				if p.Region != scope.Region {
					facts["region"] = p.Region
					return domain.Finding{Verdict: domain.VerdictFail, Reason: "REGION_MISMATCH", Facts: facts, Remediation: "rerun-region"}
				}
			}
		}
		return domain.Finding{Verdict: domain.VerdictPass, Reason: "PLACED_ON_ACCOUNT", Facts: facts}
	}
	// Storage is judged by the provider against the full StorageClass inventory.
	st, ok := prior["k8s.storage.requirements"]
	if !ok {
		return domain.Finding{Verdict: domain.VerdictUnknown, Reason: "STORAGE_EVIDENCE_UNAVAILABLE", Facts: facts}
	}
	for k, v := range st.Facts {
		facts[k] = v
	}
	return domain.Finding{Verdict: st.Verdict, Reason: st.Reason, Remediation: st.Remediation, Facts: facts}
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
