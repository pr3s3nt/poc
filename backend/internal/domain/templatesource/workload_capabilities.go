package templatesource

import "orchestrator/internal/domain/environment"

const (
	OneShotJobV1         = "one-shot-job-v1"
	TemplateDefaultsV1   = "template-defaults-v1"
	RuntimeSecurityV1    = "runtime-security-v1"
	ControllerPolicyV1   = "controller-policy-v1"
	StatefulClaimsV1     = "stateful-claims-v1"
	ContainerProbesV1    = "container-probes-v1"
	CustomExecutionV1    = "custom-execution-v1"
	ContainerLifecycleV1 = "container-lifecycle-v1"
	PodVolumesV1         = "pod-volumes-v1"
	HostPathVolumesV1    = "host-path-volumes-v1"
)

func validateCapabilities(names []string) error {
	seen := map[string]bool{}
	for _, name := range names {
		// The retired token is readable registry metadata only; it has no resolver.
		if seen[name] || (name != "workload-defaults-v1" && name != TemplateDefaultsV1 && name != RuntimeSecurityV1 && name != ControllerPolicyV1 && name != StatefulClaimsV1 && name != CustomExecutionV1 && name != ContainerLifecycleV1 && name != PodVolumesV1 && name != ContainerProbesV1 && name != HostPathVolumesV1 && name != OneShotJobV1) {
			return invalid(ManifestFile, "unsupported or duplicate workload capability")
		}
		seen[name] = true
	}
	return nil
}
func ValidateWorkloadCapabilities(m Manifest, containers map[string]environment.Container, e *environment.WorkloadExecution) error {
	available := map[string]bool{}
	for _, name := range m.WorkloadCapabilities {
		available[name] = true
	}
	if available["workload-defaults-v1"] {
		return environment.ErrExecutionUnavailable
	}
	for _, c := range containers {
		if c.SecurityContext != nil && !available[RuntimeSecurityV1] {
			return environment.ErrExecutionUnavailable
		}
		for _, p := range []*environment.Probe{c.ReadinessProbe, c.LivenessProbe, c.StartupProbe} {
			if p != nil && ((p.Type != "" && p.Type != "http") || p.Scheme != "" || p.InitialDelaySeconds != nil || p.TimeoutSeconds != nil || p.PeriodSeconds != nil || p.SuccessThreshold != nil || p.FailureThreshold != nil) && !available[ContainerProbesV1] {
				return environment.ErrExecutionUnavailable
			}
		}
		if (c.Role == "init" || c.Role == "sidecar") && !available[ContainerLifecycleV1] {
			return environment.ErrExecutionUnavailable
		}
	}
	if e == nil {
		return nil
	}
	if e.Kind == "Job" && !available[OneShotJobV1] {
		return environment.ErrExecutionUnavailable
	}
	if e.HasTemplate() {
		for _, template := range m.WorkloadTemplates {
			if template.ID == e.Template.ID && template.Version == e.Template.Version && template.RuntimeDefaults != nil && !available[TemplateDefaultsV1] {
				return environment.ErrExecutionUnavailable
			}
		}
	}
	if e.TemplateDefaults != nil && !available[TemplateDefaultsV1] {
		return environment.ErrExecutionUnavailable
	}
	if (e.PodSecurityContext != nil || e.AutomountServiceAccountToken != nil || e.ServiceAccountName != "") && !available[RuntimeSecurityV1] {
		return environment.ErrExecutionUnavailable
	}
	if (e.UpdateStrategy != nil || e.RevisionHistoryLimit != nil || e.PodDisruptionBudget != nil) && !available[ControllerPolicyV1] {
		return environment.ErrExecutionUnavailable
	}
	if (len(e.VolumeClaimTemplates) > 0 || len(e.ClaimMounts) > 0 || e.ClaimRetention != nil) && !available[StatefulClaimsV1] {
		return environment.ErrExecutionUnavailable
	}
	if !e.HasTemplate() && !available[CustomExecutionV1] {
		return environment.ErrExecutionUnavailable
	}
	for _, v := range e.Volumes {
		if v.HostPath != nil && !available[HostPathVolumesV1] {
			return environment.ErrExecutionUnavailable
		}
	}
	if (len(e.Volumes) > 0 || len(e.VolumeMounts) > 0) && !available[PodVolumesV1] {
		return environment.ErrExecutionUnavailable
	}
	return nil
}
func ExecutionInputs(m Manifest, e *environment.WorkloadExecution) (map[string]any, error) {
	if e == nil {
		return nil, nil
	}
	if !e.HasTemplate() {
		if len(e.Inputs) > 0 {
			return nil, environment.ErrExecution
		}
		return nil, nil
	}
	_, inputs, err := MatchWorkloadTemplate(m.WorkloadTemplates, e.Template.ID, e.Template.Version, e.Template.SHA256, e.Kind, e.Inputs)
	return inputs, err
}
