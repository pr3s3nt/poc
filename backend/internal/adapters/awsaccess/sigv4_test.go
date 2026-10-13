package awsaccess

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Published AWS Signature Version 4 examples (independent of this code).
func TestSignerMatchesPublishedAWSVectors(t *testing.T) {
	const secret = "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"
	when := time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC)

	// AWS General Reference: signing an IAM ListUsers request.
	canonical, signed := canonicalRequest("GET", "/", url.Values{"Action": {"ListUsers"}, "Version": {"2010-05-08"}},
		map[string]string{"content-type": "application/x-www-form-urlencoded; charset=utf-8", "host": "iam.amazonaws.com", "x-amz-date": "20150830T123600Z"}, emptySHA256)
	if signed != "content-type;host;x-amz-date" || hashHex([]byte(canonical)) != "f536975d06c0309214f805bb90ccff089219ecd68b2577efef23edd43b7e1a59" {
		t.Fatalf("canonical request hash %s", hashHex([]byte(canonical)))
	}
	if got := signature(secret, when, "us-east-1", "iam", canonical); got != "5d672d79c15b13162d9279b0855cfba6789a8edb4c82c400e06b5924a6f2b5d7" {
		t.Fatalf("IAM signature %s", got)
	}

	// The same request signed through Sign carries that signature.
	req, _ := http.NewRequest(http.MethodGet, "https://iam.amazonaws.com/?Action=ListUsers&Version=2010-05-08", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	Sign(req, nil, Credentials{AccessKeyID: "AKIDEXAMPLE", SecretAccessKey: secret}, "us-east-1", "iam", when)
	auth := req.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/iam/aws4_request, SignedHeaders=content-type;host;x-amz-date, Signature=5d672d79c15b13162d9279b0855cfba6789a8edb4c82c400e06b5924a6f2b5d7") {
		t.Fatalf("authorization %s", auth)
	}

	// aws-sig-v4-test-suite "get-vanilla".
	vanilla, _ := canonicalRequest("GET", "/", url.Values{}, map[string]string{"host": "example.amazonaws.com", "x-amz-date": "20150830T123600Z"}, emptySHA256)
	if got := signature(secret, when, "us-east-1", "service", vanilla); got != "5fa00fa31553b73ebf1942676e86291e8372ff2a2260956d9b8aae1d763fbf31" {
		t.Fatalf("get-vanilla signature %s", got)
	}
}

func TestEKSTokenShape(t *testing.T) {
	token := EKSToken(Credentials{AccessKeyID: "AKIDEXAMPLE", SecretAccessKey: "secret", SessionToken: "session"}, "us-east-1", "orc-cluster", time.Date(2026, 10, 13, 1, 2, 3, 0, time.UTC))
	rest, ok := strings.CutPrefix(token, "k8s-aws-v1.")
	if !ok || strings.ContainsAny(rest, "=+/") {
		t.Fatalf("token %q", token)
	}
	raw, err := decode(rest)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host != "sts.us-east-1.amazonaws.com" || u.Scheme != "https" {
		t.Fatalf("presigned URL %q: %v", raw, err)
	}
	q := u.Query()
	if q.Get("Action") != "GetCallerIdentity" || q.Get("X-Amz-SignedHeaders") != "host;x-k8s-aws-id" || q.Get("X-Amz-Expires") != "60" || q.Get("X-Amz-Date") != "20261013T010203Z" ||
		q.Get("X-Amz-Security-Token") != "session" || len(q.Get("X-Amz-Signature")) != 64 || !strings.HasPrefix(q.Get("X-Amz-Credential"), "AKIDEXAMPLE/20261013/us-east-1/sts/aws4_request") {
		t.Fatalf("presigned query %v", q)
	}
	// The cluster name is signed: another cluster gives another signature.
	other := EKSToken(Credentials{AccessKeyID: "AKIDEXAMPLE", SecretAccessKey: "secret", SessionToken: "session"}, "us-east-1", "other", time.Date(2026, 10, 13, 1, 2, 3, 0, time.UTC))
	if other == token {
		t.Fatal("the token does not depend on the cluster name")
	}
}
