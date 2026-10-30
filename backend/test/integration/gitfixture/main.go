// Command gitfixture serves one disposable repository over HTTPS for E2E.
// Reads are anonymous. Optional receive-pack writes require a private fixture
// credential file, allowing UI acceptance to exercise the real Git publisher.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/subtle"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/cgi"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	root := flag.String("root", "", "absolute parent of task-owned repo.git")
	listen := flag.String("listen", "172.18.0.1:19443", "explicit bridge listen address")
	certDir := flag.String("cert-dir", "", "private certificate directory outside repository")
	writerAuth := flag.String("writer-auth-file", "", "optional private file: username newline password; enables authenticated pushes")
	flag.Parse()
	if !filepath.IsAbs(*root) || !filepath.IsAbs(*certDir) {
		log.Fatal("absolute fixture directories required")
	}
	host, _, err := net.SplitHostPort(*listen)
	if err != nil || net.ParseIP(host) == nil {
		log.Fatal("explicit listen IP required")
	}
	if err := os.MkdirAll(*certDir, 0o700); err != nil {
		log.Fatal("certificate directory unavailable")
	}
	certFile, keyFile := filepath.Join(*certDir, "ca.pem"), filepath.Join(*certDir, "key.pem")
	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			log.Fatal("fixture key generation failed")
		}
		serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if err != nil {
			log.Fatal("fixture serial generation failed")
		}
		cert := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "Task-owned Git fixture"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(24 * time.Hour), IPAddresses: []net.IP{net.ParseIP(host)}, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true}
		der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
		if err != nil {
			log.Fatal("fixture certificate generation failed")
		}
		if os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600) != nil || os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0o600) != nil {
			log.Fatal("fixture certificate storage failed")
		}
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		log.Fatal("git unavailable")
	}
	out, err := exec.Command(gitPath, "--exec-path").Output()
	if err != nil {
		log.Fatal("git backend unavailable")
	}
	backend := &cgi.Handler{Path: filepath.Join(strings.TrimSpace(string(out)), "git-http-backend"), Env: []string{"GIT_PROJECT_ROOT=" + *root, "GIT_HTTP_EXPORT_ALL=1", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}}
	var username, password string
	if *writerAuth != "" {
		info, err := os.Stat(*writerAuth)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
			log.Fatal("private writer credential file required")
		}
		raw, err := os.ReadFile(*writerAuth)
		if err != nil {
			log.Fatal("writer credential unavailable")
		}
		parts := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			log.Fatal("invalid fixture writer credential")
		}
		username, password = parts[0], parts[1]
	}
	writerBackend := *backend
	writerBackend.Env = append(append([]string{}, backend.Env...), "REMOTE_USER=fixture-writer")
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Smart fetch uses POST git-upload-pack; never expose receive-pack or any
		// filesystem path beyond the one dedicated bare fixture.
		if !strings.HasPrefix(r.URL.Path, "/repo.git/") || strings.Contains(r.URL.Path, "..") {
			http.NotFound(w, r)
			return
		}
		if strings.Contains(r.URL.Path, "git-receive-pack") || r.URL.Query().Get("service") == "git-receive-pack" {
			u, p, ok := r.BasicAuth()
			if username == "" || !ok || subtle.ConstantTimeCompare([]byte(u), []byte(username)) != 1 || subtle.ConstantTimeCompare([]byte(p), []byte(password)) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="task-owned Git fixture"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
			writerBackend.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		backend.ServeHTTP(w, r)
	})
	server := &http.Server{Addr: *listen, Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second}
	fmt.Println("Task-owned HTTPS Git fixture listening on", *listen)
	if err := server.ListenAndServeTLS(certFile, keyFile); err != nil {
		log.Fatal("fixture server stopped")
	}
}
