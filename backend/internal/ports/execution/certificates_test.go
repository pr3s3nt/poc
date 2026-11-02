package execution

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func TestCDCertificateExceptionCannotCarrySecretMaterial(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	for _, tc := range []struct {
		name, value string
		extra       bool
		allowed     bool
	}{
		{"public CA", certificate, false, true},
		{"public CA chain", certificate + certificate, false, true},
		{"private key", "-----BEGIN PRIVATE KEY-----\nprivate\n-----END PRIVATE KEY-----", false, false},
		{"password disguised as CA", "password-sentinel", false, false},
		{"trailing private data", certificate + "password-sentinel", false, false},
		{"additional credential key", certificate, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := map[string]any{"ca.crt": tc.value}
			if tc.extra {
				data["token"] = "private-sentinel"
			}
			object := map[string]any{"apiVersion": "v1", "kind": "Secret", "type": "Opaque", "stringData": data}
			if PublicCertificateSecret(object) != tc.allowed {
				t.Fatal("unsafe CD certificate exception")
			}
		})
	}
	encoded := map[string]any{"apiVersion": "v1", "kind": "Secret", "type": "Opaque", "data": map[string]any{"ca.crt": base64.StdEncoding.EncodeToString([]byte(certificate))}}
	if !PublicCertificateSecret(encoded) {
		t.Fatal("public CA data rejected")
	}
	encoded["data"] = map[string]any{"ca.crt": base64.StdEncoding.EncodeToString([]byte("private-sentinel"))}
	if PublicCertificateSecret(encoded) {
		t.Fatal("base64 private value accepted as a certificate")
	}

}
