package planning

import (
	"errors"
	"testing"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
)

func labCluster(connection string) resource.Definition {
	return resource.Definition{
		Key: "cluster-lab", Name: "cluster-lab", Version: 1, ResourceTypeKey: "k8s-cluster", DriverType: resource.DriverExistingCluster,
		DriverAccount: connection,
		DriverInputs:  map[string]any{"values": map[string]any{"variables": map[string]any{"name": "${context.connection.cluster}", "kubeContext": "${context.connection.context}"}}},
		Criteria:      []resource.Criterion{{Class: "internal", ResourceID: "connections.lab"}},
	}
}

// UC-03 V02-02: the Kubernetes Definition carries no account; its legacy
// Environment binding is not an account selector or a matching filter.
func TestPlan_KubernetesDefinitionNeedsNoDriverAccount(t *testing.T) {
	req := testRequest(t, application.ProfileInternalK8s, "backend")
	for _, def := range req.Catalog.Definitions {
		if def.DriverType == resource.DriverKubernetes && def.DriverAccount != "" {
			t.Fatalf("seeded Kubernetes definition %s names an account", def.Key)
		}
	}
	if _, err := NewService(testBundles()).Plan(req); err != nil {
		t.Fatalf("plan: %v", err)
	}
}

func mutateDefinition(req *Request, key string, change func(*resource.Definition)) {
	for i := range req.Catalog.Definitions {
		if req.Catalog.Definitions[i].Key == key {
			change(&req.Catalog.Definitions[i])
		}
	}
}

func awsPins(t *testing.T, plan *Plan) map[string]tfmodule.ProviderPin {
	t.Helper()
	out := map[string]tfmodule.ProviderPin{}
	for _, contract := range plan.Terraform {
		for _, pin := range contract.ProviderBindings {
			if pin.Kind == tfmodule.BindingAWS {
				out[contract.DefinitionKey] = pin
			}
		}
	}
	return out
}

// V02: every AWS node pins its own Definition's Driver Account (key, version)
// and literal region. The Environment connection and region never decide them.
func TestPlan_AWSDefinitionsPinTheirOwnDriverAccountAndRegion(t *testing.T) {
	req := testRequest(t, application.ProfileAWSEKS, "backend")
	plan, err := NewService(testBundles()).Plan(req)
	if err != nil {
		t.Fatalf("matching account: %v", err)
	}
	pins := awsPins(t, plan)
	for _, key := range []string{"vpc-aws", "cluster-aws-eks", "postgres-aws-aurora"} {
		pin, ok := pins[key]
		if !ok || pin.Target != tfmodule.TargetAccount || pin.ConnectionKey != "aws-account" || pin.ConnectionKind != "AWS" ||
			pin.AccountVersion != 3 || pin.Region != "us-east-1" {
			t.Fatalf("%s pin = %+v", key, pin)
		}
	}
	for descriptor, match := range plan.Matches {
		if (match.DefinitionKey == "vpc-aws" || match.DefinitionKey == "cluster-aws-eks") && (match.DriverAccount != "aws-account" || match.DefinitionVersion != 1) {
			t.Fatalf("%s match = %+v", descriptor, match)
		}
	}

	// A different Environment region does not retarget an AWS node.
	other := req
	other.Env.Region = "eu-west-1"
	again, err := NewService(testBundles()).Plan(other)
	if err != nil {
		t.Fatal(err)
	}
	if got := awsPins(t, again)["vpc-aws"]; got.Region != "us-east-1" {
		t.Fatalf("environment region changed the pin: %+v", got)
	}
}

// Two AWS nodes of one plan may use different accounts and regions; no global
// account or region equality is enforced.
func TestPlan_AWSNodesMayUseDifferentAccountsAndRegions(t *testing.T) {
	req := testRequest(t, application.ProfileAWSEKS, "backend")
	req.Catalog.Accounts["aws-second"] = application.Connection{Key: "aws-second", Name: "Second", OrganizationKey: req.OrganizationKey, Kind: application.ConnectionAWS, Status: application.ConnectionReady, Version: 7}
	mutateDefinition(&req, "postgres-aws-aurora", func(d *resource.Definition) {
		d.DriverAccount = "aws-second"
		d.DriverInputs["values"].(map[string]any)["variables"].(map[string]any)["region"] = "eu-west-1"
	})
	plan, err := NewService(testBundles()).Plan(req)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	pins := awsPins(t, plan)
	if pins["vpc-aws"].ConnectionKey != "aws-account" || pins["vpc-aws"].Region != "us-east-1" {
		t.Fatalf("vpc pin = %+v", pins["vpc-aws"])
	}
	if pin := pins["postgres-aws-aurora"]; pin.ConnectionKey != "aws-second" || pin.Region != "eu-west-1" || pin.AccountVersion != 7 {
		t.Fatalf("aurora pin = %+v", pin)
	}
}

// A unique winner whose account or region is unusable stops the plan with an
// account conflict; a less specific candidate never replaces it.
func TestPlan_WinningAccountConflictFailsWithoutFallback(t *testing.T) {
	cases := map[string]func(*Request, *resource.Definition){
		"no account":      func(_ *Request, d *resource.Definition) { d.DriverAccount = "" },
		"unknown account": func(_ *Request, d *resource.Definition) { d.DriverAccount = "gone" },
		"not ready": func(r *Request, _ *resource.Definition) {
			a := r.Catalog.Accounts["aws-account"]
			a.Status = application.ConnectionVerifying
			r.Catalog.Accounts["aws-account"] = a
		},
		"internal account": func(r *Request, _ *resource.Definition) {
			a := r.Catalog.Accounts["aws-account"]
			a.Kind = application.ConnectionKubernetes
			r.Catalog.Accounts["aws-account"] = a
		},
		"foreign organization": func(r *Request, _ *resource.Definition) {
			a := r.Catalog.Accounts["aws-account"]
			a.OrganizationKey = "globex"
			r.Catalog.Accounts["aws-account"] = a
		},
	}
	for name, change := range cases {
		for _, base := range []string{"vpc-aws", "cluster-aws-eks"} {
			req := testRequest(t, application.ProfileAWSEKS, "backend")
			var winner resource.Definition
			for _, def := range req.Catalog.Definitions {
				if def.Key == base {
					winner = def
				}
			}
			// More specific than the seeded definition, so it is the unique winner.
			winner.Key, winner.Name = base+"-winner", base+" winner"
			winner.Criteria = []resource.Criterion{{ApplicationID: req.App.Key, Class: winner.Criteria[0].Class}}
			change(&req, &winner)
			req.Catalog.Definitions = append(req.Catalog.Definitions, winner)
			if _, err := NewService(testBundles()).Plan(req); !errors.Is(err, ErrAccountConflict) {
				t.Fatalf("%s/%s: want account conflict without fallback to %s, got %v", name, base, base, err)
			}
		}
	}
}

func TestPlan_AWSRegionMustBeALiteralAwsRegion(t *testing.T) {
	for name, region := range map[string]any{"missing": nil, "placeholder": "${context.env.region}", "bad shape": "mars-1", "number": 1} {
		req := testRequest(t, application.ProfileAWSEKS, "backend")
		mutateDefinition(&req, "vpc-aws", func(d *resource.Definition) {
			variables := d.DriverInputs["values"].(map[string]any)["variables"].(map[string]any)
			if region == nil {
				delete(variables, "region")
			} else {
				variables["region"] = region
			}
		})
		if _, err := NewService(testBundles()).Plan(req); err == nil {
			t.Fatalf("%s region was planned", name)
		}
	}
}

// V02-07: a Definition edit or an account change makes an older plan stale:
// the plan hash covers Definition version/content and the account version.
func TestPlan_HashFollowsDefinitionVersionAndAccount(t *testing.T) {
	req := testRequest(t, application.ProfileAWSEKS, "backend")
	base, err := NewService(testBundles()).Plan(req)
	if err != nil {
		t.Fatal(err)
	}
	renamed := req
	renamed.Catalog.Definitions = append([]resource.Definition(nil), req.Catalog.Definitions...)
	mutateDefinition(&renamed, "vpc-aws", func(d *resource.Definition) { d.Name, d.Version = "VPC mới", 2 })
	if p, err := NewService(testBundles()).Plan(renamed); err != nil || p.PlanHash == base.PlanHash {
		t.Fatalf("definition version did not change the hash: %v", err)
	}
	bumped := req
	bumped.Catalog.Accounts = map[string]application.Connection{"aws-account": req.Catalog.Accounts["aws-account"]}
	account := bumped.Catalog.Accounts["aws-account"]
	account.Version = 4
	bumped.Catalog.Accounts["aws-account"] = account
	if p, err := NewService(testBundles()).Plan(bumped); err != nil || p.PlanHash == base.PlanHash {
		t.Fatalf("account version did not change the hash: %v", err)
	}
}
