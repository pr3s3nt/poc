package v2run

import (
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"orchestrator/internal/adapters/terraform"
)

// scenarioModules are the packaged Terraform modules each AWS scenario plans
// (aws-catalog.json): the VPC, the EKS node, the namespace, Aurora, and either
// ElastiCache (A02) or the in-cluster Redis (A03).
func scenarioModules(scenario string) []string {
	if scenario == "V05" {
		// V05: EKS in the same execution with PostgreSQL and Redis running in the cluster (no managed data service).
		return []string{"vpc", "eks", "k8s-namespace", "postgres-k8s", "redis-k8s"}
	}
	mods := []string{"vpc", "eks", "k8s-namespace", "aurora"}
	if scenario == "A02" {
		return append(mods, "elasticache")
	}
	return append(mods, "redis-k8s")
}

var (
	resourceBlock = regexp.MustCompile(`(?m)^resource "([a-z0-9_]+)" "[a-zA-Z0-9_-]+" \{`)
	countVarAZ    = regexp.MustCompile(`count\s*=\s*var\.az_count\b`)
	countEBS      = regexp.MustCompile(`count\s*=\s*var\.ebs_csi_driver\s*\?\s*1\s*:\s*0`)
	azDefault     = regexp.MustCompile(`(?s)variable "az_count"\s*\{.*?default\s*=\s*(\d+)`)
)

// PlannedFromModules counts the declared resources of the scenario's shipped
// modules by Terraform type, from the embedded module sources (the very files
// the registry verifies): a block counts once, a block with `count =
// var.az_count` counts az_count times (the module default) and the EBS CSI
// blocks count once because the Definition enables the driver. This is the
// manifest's immutable planned set; the cost estimate must list exactly it.
func PlannedFromModules(scenario string) ([]Planned, error) {
	if !IsAWSScenario(scenario) {
		return nil, fmt.Errorf("planned resources exist for V05, A02 and A03 only")
	}
	modules := terraform.PackagedModules()
	counts := map[string]int{}
	for _, name := range scenarioModules(scenario) {
		raw, err := fs.ReadFile(modules, name+"/main.tf")
		if err != nil {
			return nil, err
		}
		text := string(raw)
		az := 1
		if m := azDefault.FindStringSubmatch(text); m != nil {
			az, _ = strconv.Atoi(m[1])
		}
		locs := resourceBlock.FindAllStringSubmatchIndex(text, -1)
		for i, loc := range locs {
			end := len(text)
			if i+1 < len(locs) {
				end = locs[i+1][0]
			}
			block := text[loc[0]:end]
			kind := text[loc[2]:loc[3]]
			switch {
			case countVarAZ.MatchString(block):
				counts[kind] += az
			case strings.Contains(block, "count") && !countEBS.MatchString(block) && regexp.MustCompile(`(?m)^\s+count\s*=`).MatchString(block):
				return nil, fmt.Errorf("module %s resource %s has a count expression the manifest cannot enumerate", name, kind)
			default:
				counts[kind]++
			}
		}
	}
	// The AWS acceptance runner publishes one immutable ECR repository for each
	// workload before opening the headed browser gate.
	counts["ecr_repository"] = 3
	kinds := make([]string, 0, len(counts))
	for k := range counts {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	out := make([]Planned, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, Planned{Kind: k, Count: counts[k]})
	}
	return out, nil
}

// EstimateTemplate is the skeleton of the required estimate: the identity, the
// minimum billed hours, the planned resources and every required cost kind
// with its unit left for the operator to price from a public source. It
// carries no price, no source and no ceiling.
func EstimateTemplate(c Config) (Estimate, error) {
	planned, err := PlannedFromModules(c.Scenario)
	if err != nil {
		return Estimate{}, err
	}
	e := Estimate{RunID: c.RunID, Scenario: c.Scenario, Region: AWSRegion, Hours: (c.Execution.Duration + c.Cleanup.Duration).Hours(), Resources: planned}
	for _, kind := range RequiredCostKinds(c.Scenario) {
		e.Lines = append(e.Lines, EstimateLine{Kind: kind, RetrievedAt: time.Time{}})
	}
	return e, nil
}
