package awsaccess

import "encoding/base64"

func base64RawURL(data []byte) string { return base64.RawURLEncoding.EncodeToString(data) }

func decode(s string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	return string(b), err
}
