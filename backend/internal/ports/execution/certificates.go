package execution

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"
)

// PublicCertificateSecret is the sole Secret allowed in a CD artifact. It
// contains only a public CA chain, never tokens, passwords or private keys.
func PublicCertificateSecret(object map[string]any) bool {
	if object["apiVersion"] != "v1" || object["kind"] != "Secret" || object["type"] != "Opaque" {
		return false
	}
	data, plain := object["stringData"].(map[string]any)
	if plain && object["data"] != nil {
		return false
	}
	if !plain {
		data, _ = object["data"].(map[string]any)
	}
	if len(data) != 1 {
		return false
	}
	certificate, ok := data["ca.crt"].(string)
	if !ok || strings.TrimSpace(certificate) == "" {
		return false
	}
	if !plain {
		decoded, err := base64.StdEncoding.DecodeString(certificate)
		if err != nil {
			return false
		}
		certificate = string(decoded)
	}
	remaining := []byte(strings.TrimSpace(certificate))
	count := 0
	for len(remaining) > 0 {
		if !strings.HasPrefix(string(remaining), "-----BEGIN CERTIFICATE-----") {
			return false
		}
		block, rest := pem.Decode(remaining)
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			return false
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil || !cert.IsCA {
			return false
		}
		count++
		remaining = []byte(strings.TrimSpace(string(rest)))
	}
	return count > 0
}
