package endpoint

import (
	"strings"
	"testing"
)

func validPolicy() map[string]any {
	return map[string]any{"rootDomain": "apps.example.com", "scheme": "https", "gatewayName": "public", "gatewayNamespace": "gateway-system"}
}

func TestParsePolicy(t *testing.T) {
	p, err := ParsePolicy(map[string]any{"rootDomain": "apps.example.com", "scheme": "http", "gatewayName": "public", "gatewayNamespace": "gw", "gatewaySectionName": "web", "reservedNames": []any{"www", "api"}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Scheme != "http" || p.GatewaySectionName != "web" || len(p.ReservedNames) != 2 {
		t.Fatalf("%+v", p)
	}
	for name, mutate := range map[string]func(map[string]any){
		"single label domain": func(v map[string]any) { v["rootDomain"] = "localhost" },
		"uppercase domain":    func(v map[string]any) { v["rootDomain"] = "Apps.example.com" },
		"bad scheme":          func(v map[string]any) { v["scheme"] = "ftp" },
		"missing gateway":     func(v map[string]any) { delete(v, "gatewayName") },
		"bad listener":        func(v map[string]any) { v["gatewaySectionName"] = "Web_1" },
		"non-literal":         func(v map[string]any) { v["gatewayName"] = 3 },
		"unknown field":       func(v map[string]any) { v["hostname"] = "x" },
		"bad reserved name":   func(v map[string]any) { v["reservedNames"] = []any{"a.b"} },
		"reserved not array":  func(v map[string]any) { v["reservedNames"] = "www" },
	} {
		t.Run(name, func(t *testing.T) {
			v := validPolicy()
			mutate(v)
			if _, err := ParsePolicy(v); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

func TestResolve(t *testing.T) {
	p, err := ParsePolicy(map[string]any{"rootDomain": "apps.example.com", "scheme": "https", "gatewayName": "public", "gatewayNamespace": "gw", "reservedNames": []any{"www"}})
	if err != nil {
		t.Fatal(err)
	}
	d, err := Resolve(p, map[string]any{"name": "shop", "port": "http"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Hostname != "shop.apps.example.com" || d.URL != "https://shop.apps.example.com/" || d.Path != "/" {
		t.Fatalf("%+v", d)
	}
	for name, tc := range map[string]struct {
		params map[string]any
		want   string
	}{
		"reserved":         {map[string]any{"name": "www", "port": "http"}, "reserved"},
		"uppercase":        {map[string]any{"name": "Shop", "port": "http"}, "DNS label"},
		"dotted":           {map[string]any{"name": "a.b", "port": "http"}, "DNS label"},
		"numeric port":     {map[string]any{"name": "shop", "port": "8080x"}, ""},
		"override gateway": {map[string]any{"name": "shop", "port": "http", "gatewayName": "x"}, "cannot override"},
		"query in path":    {map[string]any{"name": "shop", "port": "http", "path": "/a?b"}, "absolute"},
		"double slash":     {map[string]any{"name": "shop", "port": "http", "path": "//a"}, "absolute"},
		"parent segment":   {map[string]any{"name": "shop", "port": "http", "path": "/a/.."}, "absolute"},
		"encoded":          {map[string]any{"name": "shop", "port": "http", "path": "/a%2e"}, "absolute"},
		"non-string":       {map[string]any{"name": "shop", "port": 8080}, "string"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Resolve(p, tc.params)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
	long := Policy{RootDomain: strings.Repeat("a.", 120) + "com", Scheme: "https", GatewayName: "g", GatewayNamespace: "n"}
	if _, err := Resolve(long, map[string]any{"name": strings.Repeat("b", 20), "port": "http"}); err == nil {
		t.Fatal("over-long hostname accepted")
	}
}

func TestOwnerValidate(t *testing.T) {
	if err := (Owner{Organization: "o", Application: "a", Environment: "e", Workload: "w"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Owner{Organization: "o", Application: "a", Environment: "e"}).Validate(); err == nil {
		t.Fatal("incomplete owner accepted")
	}
}
