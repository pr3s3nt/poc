package runner

import (
	"strings"

	"orchestrator/internal/ports/terraformexec"
)

// classify maps the private stderr of a failed step to a safe class. The text
// is never returned or logged. Heuristics are ordered: a backend failure is
// reported before a credential failure, then input, then transient.
func classify(stderr string) terraformexec.Class {
	text := strings.ToLower(stderr)
	has := func(needles ...string) bool {
		for _, n := range needles {
			if strings.Contains(text, n) {
				return true
			}
		}
		return false
	}
	switch {
	case has("error acquiring the state lock", "failed to lock state", "error loading state", "failed to persist state",
		"error saving state", "failed to save state", "error releasing the state lock", "failed to retrieve state"):
		return terraformexec.ClassBackend
	case has("accessdenied", "access denied", "unauthorizedoperation", "invalidclienttokenid", "signaturedoesnotmatch",
		"expiredtoken", "authfailure", "no valid credential sources", "unauthorized", "forbidden", "invalid identity token",
		"the security token included in the request is invalid", "incorrect token", "401 ", "403 "):
		return terraformexec.ClassAuth
	case has("missing required argument", "unsupported argument", "no value for required variable", "invalid value for",
		"module not installed", "variables not allowed", "invalid function argument", "unsupported block type",
		"error: invalid", "plugin did not respond", "required plugins are not installed", "provider configuration",
		"inconsistent dependency lock file", "failed to query available provider packages", "validation"):
		return terraformexec.ClassInput
	case has("throttl", "rate exceeded", "ratelimit", "requestlimitexceeded", "toomanyrequests", "too many requests",
		"429 ", "serviceunavailable", "service unavailable", "503 ", "502 ", "504 ", "timeout", "timed out",
		"connection reset", "connection refused", "unexpected eof", "tls handshake", "temporarily unavailable",
		"requesttimeout", "request send failed", "no such host", "internalerror", "internal server error"):
		return terraformexec.ClassTransient
	}
	return terraformexec.ClassFailed
}
