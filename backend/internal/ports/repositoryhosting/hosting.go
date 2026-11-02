package repositoryhosting

import (
	"context"
	"errors"
	domain "orchestrator/internal/domain/delivery"
)

var ErrUnavailable = errors.New("repository hosting: unavailable or insufficient permission")
var ErrIdentity = errors.New("repository hosting: repository ownership or identity mismatch")

type Host interface {
	Verify(context.Context) error
	Ensure(context.Context, domain.ApplicationRepository) (domain.ApplicationRepository, error)
}
type Factory interface {
	Open(context.Context, domain.BindingVersion, domain.RepositoryPolicy) (Host, error)
}
