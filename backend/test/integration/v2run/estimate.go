package v2run

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Billable kinds of the AWS acceptance (the actual modules: public node
// subnets plus isolated private DB/cache subnets, NO NAT gateway).
const (
	CostEKSCluster    = "eks-cluster-hour"
	CostEC2Node       = "ec2-node-hour"
	CostEBSVolume     = "ebs-gb-month"
	CostPublicIPv4    = "public-ipv4-hour"
	CostAuroraACU     = "aurora-serverless-acu-hour"
	CostAuroraStorage = "aurora-storage-gb-month"
	CostElastiCache   = "elasticache-node-hour"
	CostECRStorage    = "ecr-storage-gb-month"
	CostDataTransfer  = "data-transfer-gb"
)

// RequiredCostKinds are the lines a complete estimate must price per scenario.
// A NAT gateway is deliberately absent: the VPC module creates none.
func RequiredCostKinds(scenario string) []string {
	kinds := []string{CostEKSCluster, CostEC2Node, CostEBSVolume, CostPublicIPv4, CostAuroraACU, CostAuroraStorage, CostECRStorage, CostDataTransfer}
	if scenario == "V05" {
		// No managed data service exists in V05: PostgreSQL and Redis run on the EKS node and its EBS volumes.
		kinds = []string{CostEKSCluster, CostEC2Node, CostEBSVolume, CostPublicIPv4, CostECRStorage, CostDataTransfer}
	}
	if scenario == "A02" {
		kinds = append(kinds, CostElastiCache)
	}
	sort.Strings(kinds)
	return kinds
}

// EstimateLine is one billed quantity at a public price.
type EstimateLine struct {
	Kind        string    `json:"kind"`
	Unit        string    `json:"unit"`
	PriceUSD    float64   `json:"priceUsd"`
	Quantity    float64   `json:"billedQuantity"`
	SubtotalUSD float64   `json:"subtotalUsd"`
	Source      string    `json:"priceSource"`
	RetrievedAt time.Time `json:"retrievedAt"`
	// FreeReason explains a zero price (for example a free tier is NOT assumed).
	FreeReason string `json:"freeReason,omitempty"`
}

// Estimate is the pre-run cost estimate. There is deliberately no ceiling or
// budget field: no USD limit is invented on the user's behalf.
type Estimate struct {
	RunID     string         `json:"runId"`
	Scenario  string         `json:"scenario"`
	Region    string         `json:"region"`
	Hours     float64        `json:"billedHours"`
	Lines     []EstimateLine `json:"lines"`
	TotalUSD  float64        `json:"totalUsd"`
	Resources []Planned      `json:"plannedResources"`
}

var pricingHosts = map[string]bool{"aws.amazon.com": true, "pricing.us-east-1.amazonaws.com": true, "b0.p.awsstatic.com": true}

// ParseEstimate decodes strictly: unknown fields (such as an invented
// "ceiling" or "budget") are refused.
func ParseEstimate(raw []byte) (Estimate, error) {
	var e Estimate
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&e); err != nil {
		return e, fmt.Errorf("estimate: %w", err)
	}
	return e, nil
}

// Check verifies the estimate is complete before any AWS effect: every
// required billable kind has public price, billed quantity and source; line
// and total arithmetic match; prices are recent; the estimate covers the
// manifest's planned resources and its hours cover the execution deadline plus
// the cleanup budget.
func (e Estimate) Check(c Config, m Manifest, now time.Time) error {
	var errs []string
	add := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }
	if e.RunID != c.RunID || e.Scenario != c.Scenario || e.Region != AWSRegion {
		add("estimate identity differs from the run (%s/%s/%s)", e.RunID, e.Scenario, e.Region)
	}
	minHours := (c.Execution.Duration + c.Cleanup.Duration).Hours()
	if e.Hours+1e-9 < minHours {
		add("billed hours %.2f do not cover execution deadline plus cleanup budget (%.2f)", e.Hours, minHours)
	}
	have := map[string]bool{}
	sum := 0.0
	for _, l := range e.Lines {
		have[l.Kind] = true
		if l.Quantity <= 0 || l.PriceUSD < 0 || l.Unit == "" {
			add("%s: billed quantity, unit and non-negative price are required", l.Kind)
		}
		if l.PriceUSD == 0 && l.FreeReason == "" {
			add("%s: a zero price needs a stated reason", l.Kind)
		}
		if u, err := url.Parse(l.Source); err != nil || u.Scheme != "https" || !pricingHosts[u.Host] {
			add("%s: price source must be a public AWS pricing https URL", l.Kind)
		}
		if l.RetrievedAt.IsZero() || now.Sub(l.RetrievedAt) > 7*24*time.Hour || l.RetrievedAt.After(now.Add(time.Hour)) {
			add("%s: price must have been retrieved within 7 days", l.Kind)
		}
		if math.Abs(l.PriceUSD*l.Quantity-l.SubtotalUSD) > 0.005 {
			add("%s: subtotal %.4f != price*quantity %.4f", l.Kind, l.SubtotalUSD, l.PriceUSD*l.Quantity)
		}
		sum += l.SubtotalUSD
	}
	for _, kind := range RequiredCostKinds(c.Scenario) {
		if !have[kind] {
			add("missing priced line %s", kind)
		}
	}
	if have["nat-gateway-hour"] {
		add("the VPC module has no NAT gateway; do not invent its cost")
	}
	if math.Abs(sum-e.TotalUSD) > 0.01 {
		add("total %.4f != sum of lines %.4f", e.TotalUSD, sum)
	}
	planned := map[string]int{}
	for _, p := range m.Planned {
		planned[p.Kind] = p.Count
	}
	for _, p := range e.Resources {
		if planned[p.Kind] != p.Count {
			add("planned resource %s count %d differs from the manifest (%d)", p.Kind, p.Count, planned[p.Kind])
		}
	}
	if len(e.Resources) != len(m.Planned) {
		add("estimate lists %d resource kinds, manifest %d", len(e.Resources), len(m.Planned))
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.New("estimate incomplete: " + strings.Join(errs, "; "))
}
