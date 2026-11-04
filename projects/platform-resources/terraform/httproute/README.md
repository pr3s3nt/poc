# HTTPRoute tới Gateway có sẵn

Module chỉ tạo một `gateway.networking.k8s.io/v1 HTTPRoute` trong namespace app.
Không có image/container, không tạo Gateway, controller, DNS, TLS certificate hoặc
ReferenceGrant. Xem [setup và workload declaration](../../bootstrap/HTTPROUTE.md).

Kubernetes provider cần discovery được HTTPRoute v1 CRD khi plan; dùng binding
Kubernetes account của Orchestrator. Terraform completion xác nhận object đã được
provider apply, không phải Gateway đã Accepted/ResolvedRefs.
