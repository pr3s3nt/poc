#!/usr/bin/env bash
# Full stack on an EC2 instance with an attached IAM role. Ports stay loopback;
# reach the Console through SSH port forwarding or an authenticated TLS proxy.
set -euo pipefail
repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$repo_root"
exec docker compose -f docker-compose.yml -f docker-compose.aws.yml -f docker-compose.ec2.yml up -d --build "$@"
