package kubernetes

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"orchestrator/internal/ports/execution"
	obs "orchestrator/internal/ports/observability"
)

type jobOwner struct {
	UID        string `json:"uid"`
	Kind       string `json:"kind"`
	Controller bool   `json:"controller"`
}
type inspectedJob struct {
	Metadata struct {
		Name      string     `json:"name"`
		Namespace string     `json:"namespace"`
		UID       string     `json:"uid"`
		Creation  time.Time  `json:"creationTimestamp"`
		Owners    []jobOwner `json:"ownerReferences"`
	} `json:"metadata"`
	Status struct {
		Active     int        `json:"active"`
		Started    *time.Time `json:"startTime"`
		Finished   *time.Time `json:"completionTime"`
		Conditions []struct {
			Type       string     `json:"type"`
			Status     string     `json:"status"`
			Transition *time.Time `json:"lastTransitionTime"`
		} `json:"conditions"`
	} `json:"status"`
}

func ownedJob(job inspectedJob, namespace, uid string) bool {
	if job.Metadata.Namespace != namespace || job.Metadata.UID == "" || !telemetryPodName.MatchString(job.Metadata.Name) || job.Metadata.Creation.IsZero() {
		return false
	}
	for _, owner := range job.Metadata.Owners {
		if owner.Controller && owner.Kind == "CronJob" && owner.UID == uid {
			return true
		}
	}
	return false
}
func jobView(job inspectedJob) obs.Job {
	row := obs.Job{Name: job.Metadata.Name, UID: job.Metadata.UID, CreatedAt: job.Metadata.Creation, StartedAt: job.Status.Started, State: "Pending"}
	if job.Status.Active > 0 {
		row.State = "Running"
	}
	for _, condition := range job.Status.Conditions {
		if condition.Status != "True" {
			continue
		}
		switch condition.Type {
		case "Complete":
			row.State = "Succeeded"
			row.FinishedAt = job.Status.Finished
		case "Failed":
			row.State = "Failed"
			row.FinishedAt = condition.Transition
		}
	}
	return row
}
func (r *TelemetryReader) InspectJobs(ctx context.Context, source obs.Source, ref execution.ObjectRef) obs.JobHistory {
	now := time.Now().UTC()
	if r.Now != nil {
		now = r.Now().UTC()
	}
	out := obs.JobHistory{GeneratedAt: now, Code: "UNAVAILABLE", Jobs: []obs.Job{}}
	if !source.Source.Valid() || !source.Target.CredentialBacked() || source.Target.Organization != source.OrganizationKey || !dnsLabel.MatchString(source.Target.Namespace) || ref.Kind != "CronJob" || ref.APIVersion != "batch/v1" || ref.Name != source.WorkloadID || !dnsLabel.MatchString(ref.Name) || len(ref.Owners) == 0 {
		return out
	}
	cli, cleanup, err := OpenCLI(ctx, r.KubectlPath, r.Credentials, source.Target)
	if err != nil {
		out.Code = string(telemetryCode(err))
		return out
	}
	defer cleanup()
	payload, err := cli.RunBounded(ctx, 1<<20, "get", "cronjob", ref.Name, "-n", source.Target.Namespace, "-o", "json", "--request-timeout=10s")
	if err != nil {
		out.Code = string(telemetryCode(err))
		return out
	}
	var cron map[string]any
	if json.Unmarshal(payload, &cron) != nil {
		return out
	}
	meta, _ := cron["metadata"].(map[string]any)
	uid, _ := meta["uid"].(string)
	labels := objectLabels(cron)
	validOwner := false
	for _, owner := range ref.Owners {
		validOwner = validOwner || owner != "" && labels["orchestrator.io/deployment-id"] == owner
	}
	if uid == "" || meta["namespace"] != source.Target.Namespace || meta["name"] != ref.Name || !validOwner || !sameWorkloadOwner(labels, source.ApplicationKey, source.EnvironmentKey, source.WorkloadID) {
		out.Code = "FORBIDDEN"
		return out
	}
	// Jobs inherit Pod labels, not necessarily CronJob metadata labels; join by UID.
	path := "/apis/batch/v1/namespaces/" + source.Target.Namespace + "/jobs?limit=100"
	payload, err = cli.RunBounded(ctx, 2<<20, "get", "--raw", path, "--request-timeout=10s")
	if err != nil {
		out.Code = string(telemetryCode(err))
		return out
	}
	var list struct {
		Items    []inspectedJob `json:"items"`
		Metadata struct {
			Continue string `json:"continue"`
		} `json:"metadata"`
	}
	if json.Unmarshal(payload, &list) != nil {
		return out
	}
	out.Truncated = list.Metadata.Continue != "" || len(list.Items) > 100
	for i, job := range list.Items {
		if i >= 100 {
			break
		}
		if ownedJob(job, source.Target.Namespace, uid) {
			out.Jobs = append(out.Jobs, jobView(job))
		}
	}
	sort.Slice(out.Jobs, func(i, j int) bool { return out.Jobs[i].CreatedAt.After(out.Jobs[j].CreatedAt) })
	out.Code = "OK"
	return out
}
