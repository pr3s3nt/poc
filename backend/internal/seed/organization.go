package seed

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// OrganizationIdentity is exported from the external seed input without secrets.
type OrganizationIdentity struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

var organizationKey = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

func (o OrganizationIdentity) Validate() error {
	if !organizationKey.MatchString(o.Key) || o.Name == "" || o.Name != strings.TrimSpace(o.Name) || utf8.RuneCountInString(o.Name) > 200 || !utf8.ValidString(o.Name) || strings.IndexFunc(o.Name, unicode.IsControl) >= 0 {
		return errors.New("seed: invalid Organization identity")
	}
	return nil
}

// ReadOrganizationFile refuses broad permissions, extra fields and trailing data.
func ReadOrganizationFile(path string) (OrganizationIdentity, error) {
	var o OrganizationIdentity
	f, err := os.Open(path)
	if err != nil {
		return o, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return o, err
	}
	if !stat.Mode().IsRegular() || stat.Mode().Perm()&0077 != 0 || stat.Size() > 4096 {
		return o, errors.New("seed: Organization file must be owner-only, regular and bounded")
	}
	d := json.NewDecoder(io.LimitReader(f, 4097))
	d.DisallowUnknownFields()
	if err := d.Decode(&o); err != nil {
		return o, err
	}
	if d.Decode(new(any)) != io.EOF {
		return o, errors.New("seed: trailing Organization data")
	}
	return o, o.Validate()
}
