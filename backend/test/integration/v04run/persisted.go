package v04run

import (
	"context"
	"fmt"
	"regexp"
	"sort"

	"orchestrator/test/integration/v03run"
)

// Credential MATERIAL (never a field name the product legitimately stores, such as a Terraform module output called kubeconfig)
// that must not exist in anything the product persists for a target: keys named like secrets, certificates, signed tokens,
// cluster endpoints and temporary paths.
var credentialPatterns = map[string]*regexp.Regexp{
	"credential key":      regexp.MustCompile(`"(token|kubeconfig|endpoint|caData|certificateAuthority|accessKeyId|secretAccessKey|sessionToken)"\s*:`),
	"certificate":         regexp.MustCompile(`BEGIN CERTIFICATE|BEGIN [A-Z ]*PRIVATE KEY|certificate-authority-data|client-key-data|client-certificate-data`),
	"signed EKS token":    regexp.MustCompile(`k8s-aws-v1\.`),
	"cluster endpoint":    regexp.MustCompile(`https://[A-Za-z0-9.-]+\.(eks|elb)\.amazonaws\.com`),
	"AWS access key":      regexp.MustCompile(`AKIA[0-9A-Z]{16}|ASIA[0-9A-Z]{16}`),
	"temporary file path": regexp.MustCompile(`/tmp/(orch|provider|readiness|kube)[-A-Za-z0-9_./]*`),
}

// Finding names one persisted document that carries credential material (never the material).
type Finding struct {
	Source string `json:"source"`
	Kind   string `json:"kind"`
}

// ScanPersisted reads every workload target reference and every persisted plan of the run's Console database and reports
// credential material found in them. Documents are scanned in memory and never printed.
func ScanPersisted(ctx context.Context, q v03run.Querier) (scanned int, findings []Finding, err error) {
	for _, source := range []struct{ name, sql string }{
		{"workload target", `SELECT target_ref::text FROM workload_instances`},
		{"deployment workload target", `SELECT target_ref::text FROM deployment_workloads`},
		{"deployment plan", `SELECT document::text FROM deployment_plans`},
		{"active resource outputs", `SELECT outputs::text FROM active_resources`},
	} {
		rows, qerr := q.Query(ctx, source.sql)
		if qerr != nil {
			return scanned, findings, fmt.Errorf("%s cannot be read: %w", source.name, qerr)
		}
		for rows.Next() {
			var text string
			if serr := rows.Scan(&text); serr != nil {
				rows.Close()
				return scanned, findings, fmt.Errorf("%s cannot be read", source.name)
			}
			scanned++
			for kind, pattern := range credentialPatterns {
				if pattern.MatchString(text) {
					findings = append(findings, Finding{Source: source.name, Kind: kind})
				}
			}
		}
		rows.Close()
		if rerr := rows.Err(); rerr != nil {
			return scanned, findings, fmt.Errorf("%s cannot be read", source.name)
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Source != findings[j].Source {
			return findings[i].Source < findings[j].Source
		}
		return findings[i].Kind < findings[j].Kind
	})
	return scanned, findings, nil
}

// ScanText applies the same patterns to one document (used by tests and by evidence checks).
func ScanText(text string) []string {
	var kinds []string
	for kind, pattern := range credentialPatterns {
		if pattern.MatchString(text) {
			kinds = append(kinds, kind)
		}
	}
	sort.Strings(kinds)
	return kinds
}
