package endpoint

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Intent is nonsecret render input. It authorizes one route but contains no
// manifest, reservation repository or Kubernetes client.
type Intent struct {
	Name               string `json:"name"`
	Hostname           string `json:"hostname"`
	Port               string `json:"port"`
	Path               string `json:"path"`
	GatewayName        string `json:"gatewayName"`
	GatewayNamespace   string `json:"gatewayNamespace"`
	GatewaySectionName string `json:"gatewaySectionName,omitempty"`
}

func (d Desired) Intent(owner Owner) *Intent {
	if !d.TemplateOwned {
		return nil
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s/%s/%s/%d/%s", owner.Organization, owner.Application, owner.Environment, owner.Generation, owner.Workload)))
	return &Intent{Name: "endpoint-" + hex.EncodeToString(sum[:])[:20], Hostname: d.Hostname, Port: d.Port, Path: d.Path,
		GatewayName: d.Policy.GatewayName, GatewayNamespace: d.Policy.GatewayNamespace, GatewaySectionName: d.Policy.GatewaySectionName}
}
