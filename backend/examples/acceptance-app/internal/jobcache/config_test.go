package jobcache

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func validEnv() map[string]string {
	return map[string]string{
		EnvHost: "redis.default.svc", EnvPort: "6379", EnvUsername: "acc", EnvPassword: "s3cr3t-value",
		EnvTLS: "false", EnvApp: "acceptance", EnvEnv: "dev", EnvRun: "run-01",
	}
}

func TestConfigFromEnvValid(t *testing.T) {
	cfg, err := ConfigFromEnv(env(validEnv()))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr() != "redis.default.svc:6379" || cfg.TLS || cfg.TTL != DefaultTTLSeconds*time.Second {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if got, want := cfg.KeyFor(42), "acceptance:v1:acceptance:dev:run-01:job:42"; got != want {
		t.Fatalf("key = %q, want %q", got, want)
	}
	e := validEnv()
	e[EnvTLS], e[EnvTTLSeconds] = "true", "7"
	cfg, err = ConfigFromEnv(env(e))
	if err != nil || !cfg.TLS || cfg.TTL != 7*time.Second {
		t.Fatalf("tls/ttl: %+v %v", cfg, err)
	}
}

func TestConfigFromEnvFailsClosedWithoutLeakingValues(t *testing.T) {
	mutations := map[string]map[string]string{
		"missing host":         {EnvHost: ""},
		"host with space":      {EnvHost: "bad host"},
		"host with slash":      {EnvHost: "evil/host"},
		"host too long":        {EnvHost: strings.Repeat("a", 254)},
		"missing port":         {EnvPort: ""},
		"port zero":            {EnvPort: "0"},
		"port too large":       {EnvPort: "65536"},
		"port not numeric":     {EnvPort: "63x79"},
		"missing username":     {EnvUsername: ""},
		"missing password":     {EnvPassword: ""},
		"password too long":    {EnvPassword: strings.Repeat("p", maxCredLen+1)},
		"password newline":     {EnvPassword: "a\nb"},
		"tls missing":          {EnvTLS: ""},
		"tls not a bool":       {EnvTLS: "yes"},
		"app missing":          {EnvApp: ""},
		"env with separator":   {EnvEnv: "dev:prod"},
		"run with wildcard":    {EnvRun: "run*"},
		"run too long":         {EnvRun: strings.Repeat("r", 64)},
		"ttl zero":             {EnvTTLSeconds: "0"},
		"ttl too large":        {EnvTTLSeconds: "3601"},
		"ttl not a number":     {EnvTTLSeconds: "soon"},
		"ttl negative":         {EnvTTLSeconds: "-5"},
		"app key injection":    {EnvApp: "a b"},
		"run starts with dash": {EnvRun: "-x"},
	}
	for name, m := range mutations {
		t.Run(name, func(t *testing.T) {
			e := validEnv()
			for k, v := range m {
				e[k] = v
			}
			_, err := ConfigFromEnv(env(e))
			if !errors.Is(err, ErrConfig) {
				t.Fatalf("want ErrConfig, got %v", err)
			}
			for _, v := range e {
				if len(v) > 4 && strings.Contains(err.Error(), v) {
					t.Fatalf("error leaks a configured value: %v", err)
				}
			}
			if Classify(err) != ReasonConfig {
				t.Fatalf("Classify = %q", Classify(err))
			}
		})
	}
}
