// ec2-role is an explicitly invoked AWS acceptance workload, never a unit test.
// It exercises the production scoped resolver/binder and actual Terraform AWS
// provider from an EC2 role account already registered through the Console.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"orchestrator/internal/adapters/awsaccess"
	"orchestrator/internal/adapters/terraform/providers"
	"orchestrator/internal/application/connection"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/terraformexec"
)

type reader struct{ conn application.Connection }

func (r reader) GetConnection(_ context.Context, org, key string) (application.Connection, error) {
	if org != r.conn.OrganizationKey || key != r.conn.Key {
		return application.Connection{}, errors.New("scope mismatch")
	}
	return r.conn, nil
}

func main() {
	accountFile := flag.String("account-json", "", "safe Console Account DTO (no secrets)")
	work := flag.String("work-dir", "", "private run-owned Terraform directory")
	runID := flag.String("run-id", "", "unique ownership tag")
	verifyARN := flag.String("verify-role-arn", "", "read-only EC2 role verification diagnostics")
	actions := flag.Bool("required-actions", false, "print required verification actions only")
	flag.Parse()
	if *verifyARN != "" {
		v, err := awsaccess.NewEC2RoleProvider().VerifyEC2Role(context.Background(), *verifyARN, []string{"us-east-1"})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("Read-only EC2 role verification PASS", v.PrincipalARN)
		return
	}
	if *actions {
		_ = json.NewEncoder(os.Stdout).Encode(append(awsaccess.RequiredActions(), "iam:SimulatePrincipalPolicy", "eks:ListClusters"))
		return
	}
	if *accountFile == "" || *work == "" || *runID == "" {
		panic("account-json, work-dir and run-id required")
	}
	if err := run(*accountFile, *work, *runID); err != nil {
		fmt.Fprintln(os.Stderr, "EC2 role Terraform verification failed:", err)
		os.Exit(1)
	}
}

func run(accountFile, work, runID string) (retErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	var dto struct {
		Key, Name, AuthenticationType, Status string
		Version                               int64
		Config, Verification                  map[string]any
	}
	raw, err := os.ReadFile(accountFile)
	if err != nil {
		return err
	}
	if json.Unmarshal(raw, &dto) != nil || dto.AuthenticationType != "AWS_EC2_ROLE" || dto.Key == "" || dto.Status != "READY" {
		return errors.New("expected verified EC2 account DTO")
	}
	conn := application.Connection{Key: dto.Key, Name: dto.Name, OrganizationKey: "ec2-acceptance", Kind: application.ConnectionAWS, AuthenticationType: application.AuthAWSEC2Role, Status: application.ConnectionReady, Version: dto.Version, Config: dto.Config, Verification: dto.Verification}
	// The public DTO omits the legacy default region; choose an evidence region.
	regions := conn.ConfigStrings("verificationRegions")
	if len(regions) == 0 {
		return errors.New("no evidence region")
	}
	conn.Config["region"] = regions[0]
	resolver := connection.NewCredentialResolver(reader{conn}, nil)
	resolver.SetEC2RoleProvider(awsaccess.NewEC2RoleProvider())
	binder := &providers.Binder{AWS: resolver, Kube: resolver, API: awsaccess.New()}
	env, err := binder.Bind(ctx, terraformexec.BindRequest{OrganizationKey: conn.OrganizationKey, Pins: []tfmodule.ProviderPin{{Name: "aws", Kind: tfmodule.BindingAWS, ConnectionKey: conn.Key, ConnectionKind: "AWS", Region: regions[0]}}})
	if err != nil {
		return errors.New("production provider binding failed")
	}
	defer env.Close()
	if err = os.Mkdir(work, 0700); err != nil {
		return errors.New("work directory must be fresh")
	}
	// State and plans remain owner-only recovery material until absence is proven.
	config := fmt.Sprintf(`terraform {
 required_providers {
  aws = { source = "hashicorp/aws", version = "6.60.0" }
 }
}
provider "aws" { region = %q }
resource "aws_vpc" "verification" {
 cidr_block = "10.237.0.0/24"
 tags = {
  "Name" = %q
  "project" = "orchestrator"
  "owner" = "codex"
  "environment" = "verification"
  "managed-by" = "orchestrator-ec2-role-verification"
  "run-id" = %q
  "expires-at" = %q
 }
}
output "vpc_id" { value = aws_vpc.verification.id }
`, regions[0], runID, runID, time.Now().UTC().Add(2*time.Hour).Format(time.RFC3339))
	if err = os.WriteFile(filepath.Join(work, "main.tf"), []byte(config), 0600); err != nil {
		return err
	}
	processEnv := append([]string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=" + work, "TF_IN_AUTOMATION=1", "TF_INPUT=0"}, env.Env...)
	command := func(commandCtx context.Context, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(commandCtx, "terraform", args...)
		cmd.Dir = work
		cmd.Env = processEnv
		output, err := cmd.CombinedOutput()
		if err != nil {
			_ = os.WriteFile(filepath.Join(work, "terraform-error.private"), output, 0600)
			return nil, errors.New("Terraform step failed; private recovery output retained")
		}
		return output, nil
	}
	if _, err = command(ctx, "init", "-input=false", "-no-color"); err != nil {
		return err
	}
	if _, err = command(ctx, "plan", "-out=apply.plan", "-input=false", "-no-color"); err != nil {
		return err
	}
	plan, err := command(ctx, "show", "-json", "apply.plan")
	if err != nil {
		return err
	}
	var parsed struct {
		Changes []struct {
			Address string
			Change  struct{ Actions []string }
		} `json:"resource_changes"`
	}
	if json.Unmarshal(plan, &parsed) != nil || len(parsed.Changes) != 1 || parsed.Changes[0].Address != "aws_vpc.verification" || strings.Join(parsed.Changes[0].Change.Actions, ",") != "create" {
		return errors.New("plan allowlist rejected")
	}
	// Always destroy after an apply attempt, even if the apply response is lost.
	// The independent supervisor additionally checks the exact run's VPC tag.
	defer func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 8*time.Minute)
		defer stop()
		if _, err := command(cleanupCtx, "destroy", "-auto-approve", "-input=false", "-no-color"); err != nil {
			retErr = errors.Join(retErr, errors.New("Terraform cleanup incomplete; retain recovery directory"))
		}
	}()
	if _, err = command(ctx, "apply", "-input=false", "-no-color", "apply.plan"); err != nil {
		return err
	}
	id, err := command(ctx, "output", "-raw", "vpc_id")
	if err != nil {
		return err
	}
	_ = os.WriteFile(filepath.Join(work, "vpc-id"), id, 0600)
	// Refresh explicitly through the same endpoint twice, without static env keys.
	var uri, token string
	for _, entry := range env.Env {
		if v, ok := strings.CutPrefix(entry, "AWS_CONTAINER_CREDENTIALS_FULL_URI="); ok {
			uri = v
		}
		if v, ok := strings.CutPrefix(entry, "AWS_CONTAINER_AUTHORIZATION_TOKEN="); ok {
			token = v
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := awsaccess.ReadCredentialEndpoint(ctx, uri, token, regions[0]); err != nil {
			return errors.New("credential refresh failed")
		}
	}
	fmt.Printf("Terraform EC2 role apply PASS; owned VPC %s; credential retrieval PASS; destroying now\n", strings.TrimSpace(string(id)))
	return nil
}
