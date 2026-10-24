package registry

// Shared hardened Git transport for other immutable source registries (the
// ADR-021 render bundle registry). Only validation and fetch are shared; HCL
// inspection, release identity and Terraform state stay private here.

// ValidateGitURL accepts only a credential-free https:// URL with a host and
// repository path and no query, fragment or control characters.
func ValidateGitURL(raw string) error { return validateGitURL(raw) }

// ValidGitReference reports a branch, tag or commit name that is never an
// option, refspec or revision expression.
func ValidGitReference(ref string) bool { return validGitReference(ref) }
