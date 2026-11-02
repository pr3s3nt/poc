package kubernetes

import (
	"context"
	"strings"
)

// SecretShape reads only identities and key names. Secret values are never
// returned by kubectl or exposed to the delivery observer.
type SecretShape struct {
	UID, OwnerUIDs string
	Keys           map[string]bool
}

func ReadSecretShape(ctx context.Context, cli CLI, namespace, name string) (SecretShape, error) {
	const template = `go-template={{printf "%s\n" .metadata.uid}}{{range .metadata.ownerReferences}}{{printf "%s\n" .uid}}{{end}}{{printf "--keys--\n"}}{{range $key, $value := .data}}{{printf "%s\n" $key}}{{end}}`
	raw, err := cli.Run(ctx, nil, "get", "secret", name, "-n", namespace, "--ignore-not-found=true", "-o", template)
	if err != nil || len(raw) == 0 {
		return SecretShape{}, err
	}
	parts := strings.SplitN(string(raw), "--keys--\n", 2)
	if len(parts) != 2 {
		return SecretShape{}, ErrDeliveryOwnership
	}
	identities := strings.SplitN(strings.TrimSuffix(parts[0], "\n"), "\n", 2)
	out := SecretShape{UID: identities[0], Keys: map[string]bool{}}
	if len(identities) == 2 {
		out.OwnerUIDs = identities[1]
	}
	for _, key := range strings.Fields(parts[1]) {
		out.Keys[key] = true
	}
	return out, nil
}

func (s SecretShape) OwnedBy(uid string) bool {
	for _, owner := range strings.Split(s.OwnerUIDs, "\n") {
		if uid != "" && owner == uid {
			return true
		}
	}
	return false
}
