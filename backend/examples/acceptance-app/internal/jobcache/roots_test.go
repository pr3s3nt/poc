package jobcache

import (
	"crypto/x509"
	"os"
	"os/exec"
	"testing"
)

// The workload images are FROM scratch, so there is no /etc/ssl. The test
// re-executes itself with every system root location removed and checks that
// the embedded public roots are what remains.
func TestEmbeddedPublicRootsWithoutSystemRoots(t *testing.T) {
	if os.Getenv("JOBCACHE_ROOTS_CHILD") == "1" {
		pool, err := x509.SystemCertPool()
		if err != nil {
			t.Fatal(err)
		}
		// An empty pool without embedded roots would fail every TLS verification.
		if n := len(pool.Subjects()); n < 50 { //nolint:staticcheck // counting roots is the point
			t.Fatalf("only %d trusted roots without system roots; fallback roots are not embedded", n)
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestEmbeddedPublicRootsWithoutSystemRoots$")
	cmd.Env = append(os.Environ(), "JOBCACHE_ROOTS_CHILD=1", "SSL_CERT_FILE=/nonexistent/none.pem", "SSL_CERT_DIR=/nonexistent")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child failed: %v\n%s", err, out)
	}
}
