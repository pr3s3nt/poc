// Package connectiontest builds synthetic kubeconfig documents for UC-04
// tests. All material is generated in process; no real credential is used.
package connectiontest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"
)

// Context describes one kubeconfig context with its own cluster and user.
type Context struct {
	Name, Cluster, User, Server string
	// ClusterExtra and UserExtra are YAML lines appended to the cluster and
	// user maps (indented by the builder).
	ClusterExtra, UserExtra []string
}

// Document renders a kubeconfig in YAML with one cluster and user per context.
func Document(contexts ...Context) string {
	var b strings.Builder
	b.WriteString("apiVersion: v1\nkind: Config\nclusters:\n")
	for _, c := range contexts {
		fmt.Fprintf(&b, "- name: %s\n  cluster:\n    server: %s\n", c.Cluster, c.Server)
		for _, line := range c.ClusterExtra {
			fmt.Fprintf(&b, "    %s\n", line)
		}
	}
	b.WriteString("users:\n")
	for _, c := range contexts {
		fmt.Fprintf(&b, "- name: %s\n  user:\n", c.User)
		for _, line := range c.UserExtra {
			fmt.Fprintf(&b, "    %s\n", line)
		}
	}
	b.WriteString("contexts:\n")
	for _, c := range contexts {
		fmt.Fprintf(&b, "- name: %s\n  context:\n    cluster: %s\n    user: %s\n", c.Name, c.Cluster, c.User)
	}
	if len(contexts) > 0 {
		fmt.Fprintf(&b, "current-context: %s\n", contexts[0].Name)
	}
	return b.String()
}

// TokenContext is a context authenticated by an embedded token.
func TokenContext(name, server, token string) Context {
	return Context{Name: name, Cluster: name + "-cluster", User: name + "-user", Server: server, UserExtra: []string{"token: " + token}}
}

// Single returns a one-context token kubeconfig.
func Single(name, server, token string) string { return Document(TokenContext(name, server, token)) }

var (
	once                   sync.Once
	caData, certData, keyB string
)

// TLSMaterial returns base64 CA, client certificate and client key data.
func TLSMaterial() (ca, cert, key string) {
	once.Do(func() {
		caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-ca"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
		caDER, _ := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
		clientKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		clientTemplate := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "test-user"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
		clientDER, _ := x509.CreateCertificate(rand.Reader, clientTemplate, caTemplate, &clientKey.PublicKey, caKey)
		keyDER, _ := x509.MarshalECPrivateKey(clientKey)
		caData = base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}))
		certData = base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientDER}))
		keyB = base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	})
	return caData, certData, keyB
}

// CertificateContext is a context authenticated by an embedded client
// certificate and key, with an embedded CA.
func CertificateContext(name, server string) Context {
	ca, cert, key := TLSMaterial()
	return Context{Name: name, Cluster: name + "-cluster", User: name + "-user", Server: server,
		ClusterExtra: []string{"certificate-authority-data: " + ca},
		UserExtra:    []string{"client-certificate-data: " + cert, "client-key-data: " + key}}
}
