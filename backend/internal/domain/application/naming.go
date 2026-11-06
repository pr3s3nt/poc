package application

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

var managedKeyPattern = regexp.MustCompile(`^idp-[a-z0-9-]+-[a-z2-7]{6}$`)

// NewKey freezes a readable, globally qualified IDP identity at creation.
// Reserve room for "-production" inside the provider's 49-character stem.
func NewKey(organization, name string) (string, error) {
	org, app := nameSlug(organization), nameSlug(name)
	if org == "" || app == "" {
		return "", fmt.Errorf("name must contain Latin letters or digits")
	}
	key := "idp-" + org + "-" + app
	if len(key)+7 > 38 {
		return "", fmt.Errorf("use a shorter name: the IDP application code must fit within 38 characters including the organization")
	}
	return key + "-" + strings.ToLower(rand.Text()[:6]), nil
}

// ManagedResourceName puts the purpose before the application's immutable
// random suffix, keeping that suffix at the end of every derived name.
func ManagedResourceName(key, purpose string) (string, bool) {
	if !managedKeyPattern.MatchString(key) {
		return "", false
	}
	cut := len(key) - 7
	return key[:cut] + "-" + purpose + key[cut:], true
}

func nameSlug(value string) string {
	var out strings.Builder
	separator := false
	for _, r := range norm.NFD.String(strings.ToLower(value)) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if r == 'đ' {
			r = 'd'
		}
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if separator && out.Len() > 0 {
				out.WriteByte('-')
			}
			out.WriteRune(r)
			separator = false
		} else {
			separator = true
		}
	}
	return out.String()
}
