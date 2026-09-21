// Package resource holds resource identity, contract and Active Resource state.
package resource

import (
	"fmt"
	"regexp"
	"strings"
)

var segment = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// Descriptor is the stable logical identity of a resource node: type.class#res_id.
type Descriptor struct {
	Type  string `json:"type"`
	Class string `json:"class"`
	ID    string `json:"id"`
}

// NewDescriptor validates and builds a descriptor.
func NewDescriptor(typ, class, id string) (Descriptor, error) {
	if class == "" {
		class = "default"
	}
	d := Descriptor{Type: typ, Class: class, ID: id}
	return d, d.Validate()
}

// Validate reports whether every descriptor segment is well formed.
func (d Descriptor) Validate() error {
	for name, value := range map[string]string{"type": d.Type, "class": d.Class, "id": d.ID} {
		if value == "" {
			return fmt.Errorf("resource: descriptor %s is empty", name)
		}
		if !segment.MatchString(value) {
			return fmt.Errorf("resource: descriptor %s %q is not a valid segment", name, value)
		}
	}
	if strings.Contains(d.Type, ".") {
		return fmt.Errorf("resource: descriptor type %q must not contain a dot", d.Type)
	}
	if strings.Contains(d.Class, ".") {
		return fmt.Errorf("resource: descriptor class %q must not contain a dot", d.Class)
	}
	return nil
}

// String renders the canonical descriptor form.
func (d Descriptor) String() string {
	return fmt.Sprintf("%s.%s#%s", d.Type, d.Class, d.ID)
}

// ParseDescriptor reads the canonical descriptor form.
func ParseDescriptor(s string) (Descriptor, error) {
	hash := strings.Index(s, "#")
	if hash < 0 {
		return Descriptor{}, fmt.Errorf("resource: descriptor %q has no '#'", s)
	}
	head, id := s[:hash], s[hash+1:]
	dot := strings.Index(head, ".")
	if dot < 0 {
		return Descriptor{}, fmt.Errorf("resource: descriptor %q has no type.class", s)
	}
	d := Descriptor{Type: head[:dot], Class: head[dot+1:], ID: id}
	return d, d.Validate()
}

// MarshalText lets descriptors act as JSON map keys and plain strings.
func (d Descriptor) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// UnmarshalText parses the canonical descriptor form.
func (d *Descriptor) UnmarshalText(b []byte) error {
	parsed, err := ParseDescriptor(string(b))
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}
