package environment

import (
	"fmt"
	"regexp"
)

// Annotation limits of Score metadata.annotations (ADR-024).
const (
	MaxAnnotations     = 64
	MaxAnnotationBytes = 64 << 10
)

// annotationKey is the Kubernetes annotation key syntax: an optional DNS
// subdomain prefix and a name of at most 63 characters.
var annotationKey = regexp.MustCompile(`^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?[A-Za-z0-9]([-A-Za-z0-9_.]{0,61}[A-Za-z0-9])?$`)

// ValidateAnnotations checks the per-workload annotations a Score declares.
func ValidateAnnotations(annotations map[string]string) error {
	if len(annotations) > MaxAnnotations {
		return fmt.Errorf("at most %d annotations are allowed", MaxAnnotations)
	}
	total := 0
	for key, value := range annotations {
		if len(key) > 316 || !annotationKey.MatchString(key) {
			return fmt.Errorf("annotation key %q is not a valid Kubernetes annotation key", key)
		}
		total += len(key) + len(value)
	}
	if total > MaxAnnotationBytes {
		return fmt.Errorf("annotations exceed %d bytes", MaxAnnotationBytes)
	}
	return nil
}
