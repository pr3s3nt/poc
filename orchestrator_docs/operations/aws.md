---
id: RUNBOOK-AWS
artifact: operations-runbook
status: current
last_reviewed: 2026-09-21
---

# AWS verification

## Warning

Run này tạo tài nguyên AWS có phí. Chỉ chạy khi người dùng đặt cloud verification
vào scope và đã xác nhận account, region cùng cost estimate.

## Required preflight

1. Resolve AWS identity bằng default credential chain/profile mà không in
   credential.
2. Xác nhận account ID, principal ARN và region.
3. Tạo run ID duy nhất; mọi resource phải có tối thiểu `project`, `owner`,
   `environment`, `run-id`, `expires-at`, `managed-by`.
4. Dùng Terraform state riêng trong thư mục tạm ngoài repository.
5. Chạy Pricing/Spot estimate và ghi cost ceiling.
6. Validate Terraform modules và review plan bằng allowlist/count limits.
7. Xác nhận cleanup script có đủ state/run ID trước apply.

## Automated verification

```bash
cd backend
bash test/integration/aws-verify.sh
```

Script tạo ECR tạm, build/push acceptance images, tạo VPC/EKS/Aurora, deploy ba
workload qua HTTP API, kiểm job flow/Web Console, rồi cleanup trong EXIT trap.

## Required runtime assertions

- VPC và EKS application-scoped; Aurora cung cấp shared postgres contract.
- Backend/worker nhận database outputs; secret không xuất hiện plaintext trong
  manifests, logs, state hoặc UC-09 view.
- Không tạo Service `LoadBalancer`; verification dùng port-forward.
- Workloads ready và end-to-end job flow pass.

## Cleanup order and proof

1. Dừng port-forward.
2. Xóa Kubernetes test namespace và xác nhận không còn LoadBalancer service.
3. Xóa ECR repositories/images của đúng run ID.
4. Destroy Aurora → EKS/node group → VPC bằng đúng Terraform state.
5. Query AWS API theo run ID/tag cho VPC, subnet, security group, NAT/IGW,
   EC2/EBS/EIP, route table, EKS, RDS, ECR, load balancer và IAM roles.
6. Chỉ xóa local state/kubeconfig/image sau khi query cleanup sạch.

Không dùng wildcard hoặc account/region-wide deletion. `terraform destroy`
thành công không đủ; AWS API/tag query mới là cleanup evidence cuối.
