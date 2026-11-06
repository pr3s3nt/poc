package application

import (
	"regexp"
	"strings"
	"testing"
)

func TestNewIDPKey(t *testing.T) {
	for _, tc := range []struct{ org, name, want string }{
		{"nangcap", "Order Management", "idp-nangcap-order-management"},
		{"Nâng Cấp", "  Quản lý Đơn hàng!  ", "idp-nang-cap-quan-ly-don-hang"},
		{"acme", "web__API...v2", "idp-acme-web-api-v2"},
		{"acme", "Cafe\u0301", "idp-acme-cafe"},
		{"acme", "123", "idp-acme-123"},
		{"acme", strings.Repeat("a", 22), "idp-acme-" + strings.Repeat("a", 22)},
	} {
		got, err := NewKey(tc.org, tc.name)
		if err != nil || !regexp.MustCompile(`^`+regexp.QuoteMeta(tc.want)+`-[a-z2-7]{6}$`).MatchString(got) {
			t.Fatalf("%q/%q: got %q, %v; want %q", tc.org, tc.name, got, err, tc.want)
		}
	}
	for _, name := range []string{"", "---", "💡", strings.Repeat("a", 23)} {
		if got, err := NewKey("acme", name); err == nil {
			t.Fatalf("invalid name %q accepted as %q", name, got)
		}
	}
}

func TestManagedNamesKeepTheSameSuffixAtTheEnd(t *testing.T) {
	for purpose, want := range map[string]string{
		"deploy":     "idp-acme-orders-deploy-abc234",
		"staging":    "idp-acme-orders-staging-abc234",
		"production": "idp-acme-orders-production-abc234",
	} {
		if got, ok := ManagedResourceName("idp-acme-orders-abc234", purpose); !ok || got != want {
			t.Fatalf("%s: %q %v", purpose, got, ok)
		}
	}
	for _, key := range []string{"catalog", "idp-legacy-app", "550e8400-e29b-41d4-a716-446655440000"} {
		if _, ok := ManagedResourceName(key, "deploy"); ok {
			t.Fatalf("legacy key was renamed: %s", key)
		}
	}
}
