// Package endpoint owns the public endpoint policy and stable hostname identity.
package endpoint

import (
	"fmt"
	"regexp"
	"strings"
)

var label = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

type Policy struct {
	RootDomain         string   `json:"rootDomain"`
	Scheme             string   `json:"scheme"`
	GatewayName        string   `json:"gatewayName"`
	GatewayNamespace   string   `json:"gatewayNamespace"`
	GatewaySectionName string   `json:"gatewaySectionName,omitempty"`
	ReservedNames      []string `json:"reservedNames,omitempty"`
}

type Desired struct {
	Hostname string `json:"hostname"`
	URL      string `json:"url"`
	Port     string `json:"port"`
	Path     string `json:"path"`
	Policy   Policy `json:"policy"`
}

// Claim is one reserved hostname path (EP-07).
type Claim struct {
	Hostname string `json:"hostname"`
	Path     string `json:"path"`
}

// Claim returns the reservation key of a resolved endpoint.
func (d Desired) Claim() Claim { return Claim{Hostname: d.Hostname, Path: d.Path} }

// SameHost reports whether two owners may share one hostname: the same
// Organization, Application, Environment and target generation.
func (o Owner) SameHost(other Owner) bool {
	return o.Organization == other.Organization && o.Application == other.Application && o.Environment == other.Environment && o.Generation == other.Generation
}

// Owner includes generation; moving an endpoint to another target is explicit.
type Owner struct {
	Organization string `json:"organization"`
	Application  string `json:"application"`
	Environment  string `json:"environment"`
	Generation   int64  `json:"generation"`
	Workload     string `json:"workload"`
}

func (o Owner) Validate() error {
	if o.Organization == "" || o.Application == "" || o.Environment == "" || o.Workload == "" || o.Generation < 0 {
		return fmt.Errorf("endpoint: incomplete owner")
	}
	return nil
}

func ParsePolicy(v map[string]any) (Policy, error) {
	p := Policy{}
	allowed := map[string]*string{"rootDomain": &p.RootDomain, "scheme": &p.Scheme, "gatewayName": &p.GatewayName, "gatewayNamespace": &p.GatewayNamespace, "gatewaySectionName": &p.GatewaySectionName}
	for k, value := range v {
		if dest, ok := allowed[k]; ok {
			s, ok := value.(string)
			if !ok {
				return p, fmt.Errorf("endpoint: %s must be a literal string", k)
			}
			*dest = s
			continue
		}
		if k != "reservedNames" {
			return p, fmt.Errorf("endpoint: unknown policy field %q", k)
		}
		switch names := value.(type) {
		case []string:
			p.ReservedNames = names
		case []any:
			for _, name := range names {
				s, ok := name.(string)
				if !ok {
					return p, fmt.Errorf("endpoint: reservedNames must contain DNS labels")
				}
				p.ReservedNames = append(p.ReservedNames, s)
			}
		default:
			return p, fmt.Errorf("endpoint: reservedNames must be an array")
		}
	}
	if len(p.RootDomain) > 253 || !strings.Contains(p.RootDomain, ".") {
		return p, fmt.Errorf("endpoint: rootDomain must be a lowercase DNS domain")
	}
	for _, part := range strings.Split(p.RootDomain, ".") {
		if !label.MatchString(part) {
			return p, fmt.Errorf("endpoint: invalid rootDomain")
		}
	}
	if p.Scheme != "http" && p.Scheme != "https" {
		return p, fmt.Errorf("endpoint: scheme must be http or https")
	}
	if !label.MatchString(p.GatewayName) || !label.MatchString(p.GatewayNamespace) || (p.GatewaySectionName != "" && !label.MatchString(p.GatewaySectionName)) {
		return p, fmt.Errorf("endpoint: invalid Gateway name, namespace or listener")
	}
	for _, name := range p.ReservedNames {
		if !label.MatchString(name) {
			return p, fmt.Errorf("endpoint: invalid reserved name")
		}
	}
	return p, nil
}

func Resolve(p Policy, params map[string]any) (Desired, error) {
	d := Desired{Policy: p, Path: "/"}
	for k, v := range params {
		s, ok := v.(string)
		if !ok {
			return d, fmt.Errorf("endpoint: %s must be a string", k)
		}
		switch k {
		case "name":
			d.Hostname = s
		case "port":
			d.Port = s
		case "path":
			d.Path = s
		default:
			return d, fmt.Errorf("endpoint: parameter %q cannot override routing policy", k)
		}
	}
	if !label.MatchString(d.Hostname) {
		return d, fmt.Errorf("endpoint: name must be one lowercase DNS label")
	}
	for _, name := range p.ReservedNames {
		if name == d.Hostname {
			return d, fmt.Errorf("endpoint: name is reserved by platform policy")
		}
	}
	d.Hostname += "." + p.RootDomain
	if len(d.Hostname) > 253 {
		return d, fmt.Errorf("endpoint: hostname is too long")
	}
	if !label.MatchString(d.Port) {
		return d, fmt.Errorf("endpoint: port must name a Service port")
	}
	if !strings.HasPrefix(d.Path, "/") || strings.ContainsAny(d.Path, "?#%\\\r\n\t ") || strings.Contains(d.Path, "//") || strings.Contains(d.Path, "/../") || strings.HasSuffix(d.Path, "/..") || strings.Contains(d.Path, "/./") {
		return d, fmt.Errorf("endpoint: path must be an absolute unescaped prefix")
	}
	d.URL = p.Scheme + "://" + d.Hostname + d.Path
	return d, nil
}
