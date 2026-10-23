---
id: RUNBOOK-EC2-AUTOMATION
artifact: operations-runbook
status: current
last_reviewed: 2026-10-23
---

# Automated EC2 trial deployment

## Scope and topology

This operator deployment installs the existing Orchestrator Compose stack on one
Amazon Linux 2023 x86_64 EC2 instance. Terraform manages only newly created EC2,
encrypted gp3 root disk, key pair, security groups, IAM role/profile/policy and
an optional internet-facing ALB/target group/listener. It reads an existing VPC
and public subnets without changing their routes or security groups.
This is a dev/PoC deployment using the existing seed accounts and local Vault
bootstrap, not a production authentication or high-availability design.

The user approved an HTTP ALB restricted to explicit client IPv4 CIDRs for this
trial. HTTP is not encrypted; use a trusted network and non-sensitive trial data.
Never allow 0.0.0.0/0. The instance accepts frontend traffic only from the ALB
security group; SSH only from operator CIDRs. Backend, PostgreSQL and Vault are
not public. Without ALB, use SSH forwarding to loopback port 3001. A production
HTTPS endpoint requires a separately configured domain and certificate.

The operator host also needs `flock`, `sha256sum` and `grep` for run locking
and cleanup evidence.

Host provisioning credentials belong to the operator and never enter EC2 user
data, containers or Git. The new EC2 role trusts EC2 and has an explicit action
allowlist for the current AWS modules and read-only account verification. The
current verifier simulates resource `*`, so the provisioning allowlist cannot
claim resource/tag-scoped IAM isolation. Service actions are region-limited;
IAM is global. Do not attach this role to untrusted workloads. Docker metadata
access is denied except for the backend's current container IP; IMDSv2 is required
with hop limit 2 and IPv6 metadata disabled.

## Inputs and deploy

Install AWS CLI, Terraform 1.9.8+, jq, curl, SSH and Git on the operator host.
Copy [example configuration](../../deploy/ec2/example.tfvars.json) to a private
0700 run directory outside the repo. Fill the expected account, region, existing
VPC/subnet IDs, unique deployment name, owner, expiry, exact 40-character release
commit, and explicit operator/client CIDRs. ALB needs two public subnets in
different availability zones. The deploy command generates a run-owned SSH key;
it never reads the operator's private SSH keys or AWS credential files.

```bash
./deploy/ec2/manage.sh plan /private/run/config.json /private/run
./deploy/ec2/manage.sh apply /private/run/config.json /private/run
./deploy/ec2/manage.sh status /private/run/config.json /private/run
./deploy/ec2/manage.sh tunnel /private/run/config.json /private/run
```

Plan creates no cloud resources. Review the saved plan and current prices before
apply. The apply consumes that saved plan; changes require a new plan. Private
Terraform state, plan, keys and logs remain in the run directory; do not commit
or display their contents. Keep that directory until cleanup is verified.
After health succeeds, the operator can run the positive browser/role check
(using `npm ci` in `frontend/` to install Playwright first):

```bash
node deploy/ec2/verify.mjs BASE_URL ROLE_ARN us-east-1 /private/run/accounts.png
```

It logs in through the UI, creates the EC2-role account through its form (or
compares an existing identical account), and requires READY. It performs no
infrastructure deployment. Repeat after reboot to verify persistence.
The role must be ready in the Console before using AWS Definitions. Registration
uses the existing UI/API and checks IMDS identity, regional reads and IAM
simulation; it is not a provisioning test.

Cloud-init installs Docker and checksum-verified pinned Compose, checks out the
exact release, writes non-secret runtime settings and enables a systemd service.
The service builds release-tagged images, waits for health and installs the
metadata firewall. It reuses the same Compose project/volumes on reboot.
Cloud-init has a bounded bootstrap timeout. An operator trial must start an
independent cleanup supervisor before apply, retain on successful handoff only,
and destroy on provisioning failure/deadline using this run's state. The apply
command starts [the detached supervisor](../../deploy/ec2/supervise.sh) automatically
and waits for host bootstrap/health. After browser/role/reboot verification,
write `retain-approved` in the private run directory to retain the trial. Retained
user trials are not subject to the historical issue4 immediate-success cleanup.
Record retention, expiry and costs explicitly; tags do not automatically delete
resources.

## Update, backup and recovery

On EC2, `sudo /opt/orchestrator/current/deploy/ec2/host.sh backup` stops the
runtime briefly and archives all project volumes consistently into a private
backup directory. It restarts the same release even if backup fails. Backups
include database credentials, Vault material and state encryption keys: restrict
access, encrypt off-host storage and never publish them.

`sudo /opt/orchestrator/current/deploy/ec2/host.sh update COMMIT` fetches an exact
release, builds versioned images, takes a cold backup, switches the release
symlink and restarts/health-checks the service. On failure it returns to the old
release; database schema compatibility still applies. No migration/reset or
automatic database downgrade is performed. Keep old releases, module/provider
caches and recovery volumes while resources depend on them. The same command
can select an older compatible release. Full host-loss recovery and automated
backup restoration remain manual; a local volume backup alone is not disaster
recovery. Check free disk space and copy backups off-host privately.

## Cleanup and native verification

First remove managed application/cloud resources through Orchestrator and prove
native absence. Back up any retained data. Then explicitly run:

```bash
./deploy/ec2/manage.sh destroy /private/run/config.json /private/run
```

The command checks account identity and the exact instance run-id tag before
Terraform destroy. It never deletes by account-wide/prefix search. The root disk
is deleted with the owned instance, so destroy requires explicit operator intent.
After destroy run `./deploy/ec2/absence.sh /private/run/config.json /private/run`
to verify exact EC2, volume, ALB, target-group, security-group, key-pair,
IAM role and instance-profile absence using native APIs and saved resource IDs.
An empty Terraform state alone is not cleanup proof. Retain recovery material if
ownership or cleanup is uncertain.
