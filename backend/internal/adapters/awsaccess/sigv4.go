// Package awsaccess is a minimal AWS Signature Version 4 client for the three
// calls the Terraform executor needs with an explicit, scoped access key:
// the account check (STS GetCallerIdentity), the EKS cluster endpoint and CA
// (EKS DescribeCluster) and the short-lived EKS authentication token. It
// never reads the host credential chain, profile, config files or instance
// metadata.
package awsaccess

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Credentials are an explicit AWS access key (and optional session token).
type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

const (
	algorithm   = "AWS4-HMAC-SHA256"
	timeFormat  = "20060102T150405Z"
	dateFormat  = "20060102"
	emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

func hashHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func signingKey(secret, date, region, service string) []byte {
	k := hmacSHA256([]byte("AWS4"+secret), date)
	k = hmacSHA256(k, region)
	k = hmacSHA256(k, service)
	return hmacSHA256(k, "aws4_request")
}

// encode percent-encodes everything but the RFC 3986 unreserved characters.
func encode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		default:
			b.WriteString("%" + strings.ToUpper(hex.EncodeToString([]byte{c})))
		}
	}
	return b.String()
}

func canonicalQuery(values url.Values) string {
	pairs := make([]string, 0, len(values))
	for key, list := range values {
		for _, v := range list {
			pairs = append(pairs, encode(key)+"="+encode(v))
		}
	}
	sort.Strings(pairs)
	return strings.Join(pairs, "&")
}

func canonicalPath(path string) string {
	if path == "" {
		return "/"
	}
	segments := strings.Split(path, "/")
	for i, s := range segments {
		segments[i] = encode(s)
	}
	return strings.Join(segments, "/")
}

// canonicalRequest renders the canonical request and the signed header list.
func canonicalRequest(method, path string, query url.Values, headers map[string]string, payloadHash string) (string, string) {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	var canonical strings.Builder
	for _, name := range names {
		canonical.WriteString(name + ":" + strings.Join(strings.Fields(headers[name]), " ") + "\n")
	}
	signed := strings.Join(names, ";")
	return strings.Join([]string{method, canonicalPath(path), canonicalQuery(query), canonical.String(), signed, payloadHash}, "\n"), signed
}

func stringToSign(t time.Time, region, service, canonical string) string {
	scope := t.Format(dateFormat) + "/" + region + "/" + service + "/aws4_request"
	return strings.Join([]string{algorithm, t.Format(timeFormat), scope, hashHex([]byte(canonical))}, "\n")
}

// signature computes the SigV4 signature of a canonical request.
func signature(secret string, t time.Time, region, service, canonical string) string {
	key := signingKey(secret, t.Format(dateFormat), region, service)
	return hex.EncodeToString(hmacSHA256(key, stringToSign(t, region, service, canonical)))
}

// Sign signs req with the Authorization header. The signed headers are the
// host, content-type (when set), x-amz-date and x-amz-security-token (when a
// session token is given).
func Sign(req *http.Request, body []byte, c Credentials, region, service string, t time.Time) {
	t = t.UTC()
	req.Header.Set("X-Amz-Date", t.Format(timeFormat))
	if c.SessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", c.SessionToken)
	}
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	headers := map[string]string{"host": host, "x-amz-date": t.Format(timeFormat)}
	if v := req.Header.Get("Content-Type"); v != "" {
		headers["content-type"] = v
	}
	if c.SessionToken != "" {
		headers["x-amz-security-token"] = c.SessionToken
	}
	canonical, signed := canonicalRequest(req.Method, req.URL.EscapedPath(), req.URL.Query(), headers, hashHex(body))
	scope := t.Format(dateFormat) + "/" + region + "/" + service + "/aws4_request"
	req.Header.Set("Authorization", algorithm+" Credential="+c.AccessKeyID+"/"+scope+", SignedHeaders="+signed+", Signature="+signature(c.SecretAccessKey, t, region, service, canonical))
}

// EKSToken returns the short-lived bearer token that authenticates to an EKS
// cluster: "k8s-aws-v1." plus the unpadded URL-safe base64 of a presigned STS
// GetCallerIdentity URL that signs the cluster name header. It is valid for
// minutes; callers derive it per invocation and never store it.
func EKSToken(c Credentials, region, cluster string, now time.Time) string {
	now = now.UTC()
	host := "sts." + region + ".amazonaws.com"
	scope := now.Format(dateFormat) + "/" + region + "/sts/aws4_request"
	query := url.Values{
		"Action":              {"GetCallerIdentity"},
		"Version":             {"2011-06-15"},
		"X-Amz-Algorithm":     {algorithm},
		"X-Amz-Credential":    {c.AccessKeyID + "/" + scope},
		"X-Amz-Date":          {now.Format(timeFormat)},
		"X-Amz-Expires":       {"60"},
		"X-Amz-SignedHeaders": {"host;x-k8s-aws-id"},
	}
	if c.SessionToken != "" {
		query.Set("X-Amz-Security-Token", c.SessionToken)
	}
	canonical, _ := canonicalRequest("GET", "/", query, map[string]string{"host": host, "x-k8s-aws-id": cluster}, emptySHA256)
	sig := signature(c.SecretAccessKey, now, region, "sts", canonical)
	presigned := "https://" + host + "/?" + canonicalQuery(query) + "&X-Amz-Signature=" + sig
	return "k8s-aws-v1." + base64RawURL([]byte(presigned))
}
