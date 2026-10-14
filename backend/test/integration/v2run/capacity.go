package v2run

import (
	"fmt"
	"sort"
	"strings"
)

// Instance describes one node instance type for the pod-capacity gate. The
// values are the public EC2 network limits (maxENI, IPv4 per ENI) and sizes.
type Instance struct {
	Name       string
	Arch       string
	VCPU       int
	MemoryMiB  int
	MaxENI     int
	IPv4PerENI int
}

// KnownInstances are the candidate small instance types of the acceptance.
var KnownInstances = map[string]Instance{
	"t4g.small":  {Name: "t4g.small", Arch: "arm64", VCPU: 2, MemoryMiB: 2048, MaxENI: 3, IPv4PerENI: 4},
	"t4g.medium": {Name: "t4g.medium", Arch: "arm64", VCPU: 2, MemoryMiB: 4096, MaxENI: 3, IPv4PerENI: 6},
	"t3.small":   {Name: "t3.small", Arch: "amd64", VCPU: 2, MemoryMiB: 2048, MaxENI: 3, IPv4PerENI: 4},
	"t3.medium":  {Name: "t3.medium", Arch: "amd64", VCPU: 2, MemoryMiB: 4096, MaxENI: 3, IPv4PerENI: 6},
}

// PodBudget is the EKS max-pods of the standard VPC CNI: maxENI*(IPv4-1)+2.
// The +2 are the hostNetwork aws-node and kube-proxy pods, which kubelet still
// counts against its pod limit.
func (i Instance) PodBudget() int { return i.MaxENI*(i.IPv4PerENI-1) + 2 }

// AllocatableCPUm is the node CPU after the EKS kube-reserved formula
// (6% of the first core, 1% of the second, 0.5% of cores three and four,
// 0.25% above).
func (i Instance) AllocatableCPUm() int {
	reserved := 0.0
	for core := 1; core <= i.VCPU; core++ {
		switch {
		case core == 1:
			reserved += 60
		case core == 2:
			reserved += 10
		case core <= 4:
			reserved += 5
		default:
			reserved += 2.5
		}
	}
	return i.VCPU*1000 - int(reserved)
}

// AllocatableMemoryMiB is node memory after kube-reserved (255 + 11*maxPods
// MiB) and the 100 MiB hard eviction threshold.
func (i Instance) AllocatableMemoryMiB() int {
	return i.MemoryMiB - (255 + 11*i.PodBudget()) - 100
}

// Pod is a recurring demand on the node count of the cluster.
type Pod struct {
	Name     string
	Replicas int
	// PerNode marks a DaemonSet: one replica on every node.
	PerNode bool
	// Counted is false for the pods already inside the "+2" of the budget.
	Counted  bool
	CPUm     int
	MemoryMi int
}

// EKSSystemPods are the pods of the actual EKS module: the default add-ons
// (vpc-cni, kube-proxy, coredns x2) and the two add-ons the module installs
// (eks-pod-identity-agent, aws-ebs-csi-driver with its controller x2 and node
// DaemonSet). Requests are the add-on defaults.
func EKSSystemPods() []Pod {
	return []Pod{
		{Name: "aws-node", PerNode: true, Replicas: 1, Counted: false, CPUm: 25},
		{Name: "kube-proxy", PerNode: true, Replicas: 1, Counted: false, CPUm: 100},
		{Name: "coredns", Replicas: 2, Counted: true, CPUm: 100, MemoryMi: 70},
		{Name: "eks-pod-identity-agent", PerNode: true, Replicas: 1, Counted: true},
		{Name: "ebs-csi-controller", Replicas: 2, Counted: true, CPUm: 60, MemoryMi: 240},
		{Name: "ebs-csi-node", PerNode: true, Replicas: 1, Counted: true, CPUm: 30, MemoryMi: 120},
	}
}

// AcceptancePods are the three workloads (Score requests of the acceptance
// documents) plus the in-cluster Redis when the scenario has one.
func AcceptancePods(redis bool) []Pod {
	pods := []Pod{
		{Name: "backend", Replicas: 1, Counted: true, CPUm: 50, MemoryMi: 64},
		{Name: "worker", Replicas: 1, Counted: true, MemoryMi: 48},
		{Name: "frontend", Replicas: 1, Counted: true},
	}
	if redis {
		pods = append(pods, Pod{Name: "redis", Replicas: 1, Counted: true, CPUm: 100, MemoryMi: 128})
	}
	return pods
}

// CapacityResult is the outcome of the gate.
type CapacityResult struct {
	Instance                         string
	Nodes                            int
	PodBudget, PodsRequired          int
	CPUAllocatable, CPURequested     int
	MemoryAllocatable, MemoryRequest int
	Failures                         []string
}

// OK reports whether the sizing fits.
func (r CapacityResult) OK() bool { return len(r.Failures) == 0 }

// CheckCapacity fails (before any create) when the pods, CPU or memory
// requests of the system add-ons plus the workloads exceed what `nodes` nodes
// of the instance type provide. pods is the union of EKSSystemPods and the
// scenario pods.
func CheckCapacity(inst Instance, nodes int, pods []Pod) CapacityResult {
	r := CapacityResult{Instance: inst.Name, Nodes: nodes}
	r.PodBudget = nodes * inst.PodBudget()
	r.CPUAllocatable, r.MemoryAllocatable = nodes*inst.AllocatableCPUm(), nodes*inst.AllocatableMemoryMiB()
	// The "+2" aws-node/kube-proxy pods are already inside the budget.
	for _, p := range pods {
		n := p.Replicas
		if p.PerNode {
			n = nodes * p.Replicas
		}
		r.CPURequested += n * p.CPUm
		r.MemoryRequest += n * p.MemoryMi
		if p.Counted {
			r.PodsRequired += n
		}
	}
	r.PodBudget -= nodes * 2
	if nodes <= 0 {
		r.Failures = append(r.Failures, "no nodes")
	}
	if r.PodsRequired > r.PodBudget {
		r.Failures = append(r.Failures, fmt.Sprintf("pods: %d required > %d schedulable (%s max-pods %d per node minus the aws-node and kube-proxy pods)", r.PodsRequired, r.PodBudget, inst.Name, inst.PodBudget()))
	}
	if r.CPURequested > r.CPUAllocatable {
		r.Failures = append(r.Failures, fmt.Sprintf("cpu: %dm requested > %dm allocatable", r.CPURequested, r.CPUAllocatable))
	}
	if r.MemoryRequest > r.MemoryAllocatable {
		r.Failures = append(r.Failures, fmt.Sprintf("memory: %dMi requested > %dMi allocatable", r.MemoryRequest, r.MemoryAllocatable))
	}
	sort.Strings(r.Failures)
	return r
}

// CheckAWSScenario applies the gate to the AWS scenarios: A02 (managed Redis
// and PostgreSQL: three workloads) and A03 (Redis in cluster: four pods). The
// EKS module runs one node (desired size 1).
func CheckAWSScenario(scenario, instanceType string) (CapacityResult, error) {
	inst, ok := KnownInstances[instanceType]
	if !ok {
		return CapacityResult{}, fmt.Errorf("unknown instance type %q (known: %s)", instanceType, strings.Join(knownNames(), ", "))
	}
	if scenario != "A02" && scenario != "A03" {
		return CapacityResult{}, fmt.Errorf("capacity gate applies to A02 and A03, not %q", scenario)
	}
	pods := append(EKSSystemPods(), AcceptancePods(scenario == "A03")...)
	res := CheckCapacity(inst, 1, pods)
	return res, nil
}

func knownNames() []string {
	names := make([]string, 0, len(KnownInstances))
	for k := range KnownInstances {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
