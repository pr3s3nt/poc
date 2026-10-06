// Package credentials defines the UC-04 Connection credential store port. It is
// distinct from the UC-12 configuration Provider and the UC-08 output-secret
// store: values are scoped to one Organization and Connection, references are
// opaque and immutable, and no public read operation exists.
package credentials

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

// Errors returned by credential store adapters. Messages never carry values,
// tokens or remote response bodies.
var (
	// ErrUnavailable means no durable credential store is configured or the
	// configured store cannot be reached.
	ErrUnavailable = errors.New("credentials: connection credential store is unavailable")
	// ErrInvalidReference rejects a reference outside the requested scope.
	ErrInvalidReference = errors.New("credentials: reference is outside the connection scope")
	// ErrNotFound means the scoped object does not exist.
	ErrNotFound = errors.New("credentials: connection credential not found")
)

// Store keeps Connection credential bytes outside the database.
//
// Put writes a new immutable object and returns its opaque reference. When
// Put fails after the reference was chosen it still returns the reference so
// the caller can roll the attempt back with Delete. Get and Delete validate
// the complete Organization/Connection scope before any remote call.
type Store interface {
	Put(ctx context.Context, organizationKey, connectionKey string, value []byte) (string, error)
	Get(ctx context.Context, organizationKey, connectionKey, reference string) ([]byte, error)
	Delete(ctx context.Context, organizationKey, connectionKey, reference string) error
	// Durable reports whether stored objects survive a process restart.
	Durable() bool
}

var (
	scopeSegment  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	objectSegment = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	mountSegment  = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

// ValidScope reports whether Organization and Connection keys are safe path
// segments.
func ValidScope(organizationKey, connectionKey string) bool {
	return scopeSegment.MatchString(organizationKey) && scopeSegment.MatchString(connectionKey)
}

// ValidMount reports whether a KV mount name is a safe path segment.
func ValidMount(mount string) bool { return mountSegment.MatchString(mount) }

// ObjectPath returns the store path of one credential object.
func ObjectPath(organizationKey, connectionKey, objectID string) string {
	return "orchestrator/connections/" + organizationKey + "/" + connectionKey + "/credentials/" + objectID
}

// Reference renders the opaque reference of one object.
func Reference(scheme, mount, organizationKey, connectionKey, objectID string) string {
	return scheme + "://" + mount + "/" + ObjectPath(organizationKey, connectionKey, objectID)
}

// ParseReference validates that reference names exactly one object of the
// given Organization and Connection under scheme/mount, and returns its path.
// Path traversal, other scopes, other schemes and arbitrary URLs are rejected.
func ParseReference(reference, scheme, mount, organizationKey, connectionKey string) (string, error) {
	if !ValidScope(organizationKey, connectionKey) || !ValidMount(mount) {
		return "", ErrInvalidReference
	}
	path, ok := strings.CutPrefix(reference, scheme+"://"+mount+"/")
	if !ok {
		return "", ErrInvalidReference
	}
	segments := strings.Split(path, "/")
	if len(segments) != 6 || segments[0] != "orchestrator" || segments[1] != "connections" ||
		segments[2] != organizationKey || segments[3] != connectionKey || segments[4] != "credentials" ||
		!objectSegment.MatchString(segments[5]) {
		return "", ErrInvalidReference
	}
	return path, nil
}
