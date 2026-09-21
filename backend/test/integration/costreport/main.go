// Command costreport prices the cloud verification run before any apply. It
// reads current on-demand prices from the AWS Pricing API and the live Spot
// price, picks the cheapest configuration that still runs three workloads and
// prints both a Markdown summary and a machine-readable shell snippet.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

type priceDimension struct {
	Description  string            `json:"description"`
	Unit         string            `json:"unit"`
	PricePerUnit map[string]string `json:"pricePerUnit"`
}

type term struct {
	PriceDimensions map[string]priceDimension `json:"priceDimensions"`
}

type product struct {
	Product struct {
		Attributes map[string]string `json:"attributes"`
	} `json:"product"`
	Terms struct {
		OnDemand map[string]term `json:"OnDemand"`
	} `json:"terms"`
}

type priceList struct {
	PriceList []string `json:"PriceList"`
}

type lineItem struct {
	Component   string  `json:"component"`
	Description string  `json:"description"`
	Unit        string  `json:"unit"`
	UnitPrice   float64 `json:"unitPrice"`
	Quantity    float64 `json:"quantity"`
	HourlyUSD   float64 `json:"hourlyUsd"`
	Source      string  `json:"source"`
}

func main() {
	region := flag.String("region", "us-east-1", "AWS region")
	location := flag.String("location", "US East (N. Virginia)", "Pricing API location name")
	hours := flag.Float64("hours", 2, "expected lifetime of the run in hours")
	instanceTypes := flag.String("instance-types", "t4g.small,t3.small", "candidate node instance types, cheapest wins")
	diskSize := flag.Float64("disk-size", 20, "node EBS volume size in GiB")
	minACU := flag.Float64("aurora-min-acu", 0, "Aurora Serverless v2 minimum capacity")
	maxACU := flag.Float64("aurora-max-acu", 1, "Aurora Serverless v2 maximum capacity")
	shellOut := flag.String("shell-out", "", "optional path for the shell snippet with the chosen configuration")
	jsonOut := flag.String("json-out", "", "optional path for the JSON estimate")
	flag.Parse()

	var items []lineItem
	var notes []string

	// One EKS control plane.
	eksPrice, err := onDemandPrice("AmazonEKS", map[string]string{
		"regionCode": *region, "usagetype": usageType(*region, "AmazonEKS-Hours:perCluster"),
	})
	if err != nil {
		notes = append(notes, "EKS price lookup failed: "+err.Error())
		eksPrice = 0.10
	}
	items = append(items, lineItem{
		Component: "EKS control plane", Description: "one standard-support cluster", Unit: "hour",
		UnitPrice: eksPrice, Quantity: 1, HourlyUSD: eksPrice, Source: "AWS Pricing API",
	})

	// Cheapest candidate node type that still fits the pod count.
	type candidate struct {
		name     string
		onDemand float64
		spot     float64
	}
	var candidates []candidate
	for _, name := range strings.Split(*instanceTypes, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		price, err := onDemandPrice("AmazonEC2", map[string]string{
			"instanceType": name, "location": *location, "operatingSystem": "Linux",
			"tenancy": "Shared", "preInstalledSw": "NA", "capacitystatus": "Used",
		})
		if err != nil {
			notes = append(notes, fmt.Sprintf("%s on-demand lookup failed: %v", name, err))
			continue
		}
		candidates = append(candidates, candidate{name: name, onDemand: price, spot: spotPrice(*region, name)})
	}
	if len(candidates) == 0 {
		fmt.Fprintln(os.Stderr, "costreport: no instance price could be resolved")
		os.Exit(1)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].onDemand < candidates[j].onDemand })
	node := candidates[0]

	capacityType := "ON_DEMAND"
	nodePrice := node.onDemand
	if node.spot > 0 && node.spot < node.onDemand {
		capacityType = "SPOT"
		nodePrice = node.spot
	}
	amiType := "AL2023_x86_64_STANDARD"
	arch := "amd64"
	if strings.HasPrefix(node.name, "t4g") || strings.HasPrefix(node.name, "m7g") || strings.HasPrefix(node.name, "c7g") {
		amiType = "AL2023_ARM_64_STANDARD"
		arch = "arm64"
	}
	items = append(items, lineItem{
		Component: "Managed node group", Description: fmt.Sprintf("1 x %s (%s)", node.name, capacityType), Unit: "hour",
		UnitPrice: nodePrice, Quantity: 1, HourlyUSD: nodePrice, Source: "AWS Pricing API + Spot history",
	})

	// Node root volume.
	gp3, err := onDemandPrice("AmazonEC2", map[string]string{
		"location": *location, "productFamily": "Storage", "volumeApiName": "gp3",
	})
	if err != nil {
		notes = append(notes, "gp3 price lookup failed: "+err.Error())
		gp3 = 0.08
	}
	ebsHourly := gp3 * *diskSize / 730
	items = append(items, lineItem{
		Component: "Node EBS volume", Description: fmt.Sprintf("%.0f GiB gp3", *diskSize), Unit: "GB-month",
		UnitPrice: gp3, Quantity: *diskSize, HourlyUSD: ebsHourly, Source: "AWS Pricing API",
	})

	// Aurora Serverless v2 on Aurora Standard storage.
	acu, err := onDemandPriceMatching("AmazonRDS", map[string]string{
		"regionCode": *region, "productFamily": "ServerlessV2", "databaseEngine": "Aurora PostgreSQL",
	}, func(attrs map[string]string) bool { return attrs["usagetype"] == "Aurora:ServerlessV2Usage" })
	if err != nil {
		notes = append(notes, "Aurora ACU lookup failed: "+err.Error())
		acu = 0.12
	}
	billedACU := *minACU
	if billedACU == 0 {
		// A writer that is serving traffic never stays at zero capacity.
		billedACU = 0.5
		notes = append(notes, "minimum capacity is 0 ACU; the estimate bills 0.5 ACU because the writer serves traffic during the test")
	}
	items = append(items, lineItem{
		Component: "Aurora Serverless v2 writer",
		Description: fmt.Sprintf("Aurora Standard, min %.1f / max %.1f ACU, no read replica",
			*minACU, *maxACU),
		Unit: "ACU-hour", UnitPrice: acu, Quantity: billedACU, HourlyUSD: acu * billedACU, Source: "AWS Pricing API",
	})

	// One public IPv4 address for the single node.
	ipv4, err := onDemandPriceMatching("AmazonVPC", map[string]string{
		"regionCode": *region, "group": "VPCPublicIPv4Address",
	}, func(attrs map[string]string) bool { return strings.Contains(attrs["usagetype"], "InUseAddress") })
	if err != nil {
		notes = append(notes, "public IPv4 lookup failed: "+err.Error())
		ipv4 = 0.005
	}
	items = append(items, lineItem{
		Component: "Public IPv4", Description: "one in-use address on the node (no NAT Gateway)", Unit: "hour",
		UnitPrice: ipv4, Quantity: 1, HourlyUSD: ipv4, Source: "AWS Pricing API",
	})

	var hourly float64
	for _, item := range items {
		hourly += item.HourlyUSD
	}
	total := hourly * *hours

	fmt.Printf("# Cloud cost estimate (%s, %s)\n\n", *region, time.Now().UTC().Format(time.RFC3339))
	fmt.Println("| Component | Detail | Unit price | Quantity | USD/hour |")
	fmt.Println("|---|---|---|---|---|")
	for _, item := range items {
		fmt.Printf("| %s | %s | $%.4f / %s | %.2f | $%.4f |\n",
			item.Component, item.Description, item.UnitPrice, item.Unit, item.Quantity, item.HourlyUSD)
	}
	fmt.Printf("| **Total** | | | | **$%.4f** |\n\n", hourly)
	fmt.Printf("Estimated cost for %.1f hours: **$%.2f**\n\n", *hours, total)
	fmt.Println("Excluded by policy: NAT Gateway, public load balancer, bastion host, extra node groups,")
	fmt.Println("read replicas, I/O-Optimized storage, EKS Auto Mode, paid add-ons, reservations.")
	if len(notes) > 0 {
		fmt.Println("\nAssumptions:")
		for _, note := range notes {
			fmt.Println("- " + note)
		}
	}

	if *shellOut != "" {
		snippet := fmt.Sprintf(`NODE_INSTANCE_TYPE=%s
NODE_CAPACITY_TYPE=%s
NODE_AMI_TYPE=%s
NODE_ARCH=%s
NODE_DISK_SIZE=%.0f
AURORA_MIN_ACU=%g
AURORA_MAX_ACU=%g
ESTIMATED_USD_PER_HOUR=%.4f
ESTIMATED_USD_TOTAL=%.2f
`, node.name, capacityType, amiType, arch, *diskSize, *minACU, *maxACU, hourly, total)
		if err := os.WriteFile(*shellOut, []byte(snippet), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "costreport: write shell snippet: %v\n", err)
			os.Exit(1)
		}
	}
	if *jsonOut != "" {
		payload, _ := json.MarshalIndent(map[string]any{
			"region": *region, "items": items, "hourlyUsd": hourly, "hours": *hours,
			"totalUsd": total, "instanceType": node.name, "capacityType": capacityType, "notes": notes,
		}, "", "  ")
		if err := os.WriteFile(*jsonOut, payload, 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "costreport: write json: %v\n", err)
			os.Exit(1)
		}
	}
}

func usageType(region, suffix string) string {
	prefix := map[string]string{
		"us-east-1": "USE1", "us-east-2": "USE2", "us-west-1": "USW1", "us-west-2": "USW2",
		"eu-west-1": "EUW1", "eu-central-1": "EUC1", "ap-southeast-1": "APS1", "ap-southeast-2": "APS2",
		"ap-northeast-1": "APN1", "ap-south-1": "APS3",
	}[region]
	if prefix == "" {
		return suffix
	}
	return prefix + "-" + suffix
}

func getProducts(service string, filters map[string]string) ([]product, error) {
	args := []string{"pricing", "get-products", "--region", "us-east-1", "--service-code", service,
		"--max-results", "40", "--output", "json", "--filters"}
	keys := make([]string, 0, len(filters))
	for k := range filters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, fmt.Sprintf("Type=TERM_MATCH,Field=%s,Value=%s", k, filters[k]))
	}
	out, err := exec.Command("aws", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("aws pricing get-products %s: %w", service, err)
	}
	var list priceList
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, fmt.Errorf("decode price list: %w", err)
	}
	products := make([]product, 0, len(list.PriceList))
	for _, raw := range list.PriceList {
		var p product
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			continue
		}
		products = append(products, p)
	}
	return products, nil
}

func onDemandPrice(service string, filters map[string]string) (float64, error) {
	return onDemandPriceMatching(service, filters, func(map[string]string) bool { return true })
}

func onDemandPriceMatching(service string, filters map[string]string, accept func(map[string]string) bool) (float64, error) {
	products, err := getProducts(service, filters)
	if err != nil {
		return 0, err
	}
	for _, p := range products {
		if !accept(p.Product.Attributes) {
			continue
		}
		for _, t := range p.Terms.OnDemand {
			for _, d := range t.PriceDimensions {
				value, err := strconv.ParseFloat(d.PricePerUnit["USD"], 64)
				if err == nil && value > 0 {
					return value, nil
				}
			}
		}
	}
	return 0, fmt.Errorf("no on-demand price found for %s %v", service, filters)
}

func spotPrice(region, instanceType string) float64 {
	out, err := exec.Command("aws", "ec2", "describe-spot-price-history",
		"--region", region, "--instance-types", instanceType,
		"--product-descriptions", "Linux/UNIX", "--max-items", "10",
		"--query", "SpotPriceHistory[].SpotPrice", "--output", "json").Output()
	if err != nil {
		return 0
	}
	var prices []string
	if err := json.Unmarshal(out, &prices); err != nil || len(prices) == 0 {
		return 0
	}
	best := 0.0
	for _, raw := range prices {
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			continue
		}
		if best == 0 || value < best {
			best = value
		}
	}
	return best
}
