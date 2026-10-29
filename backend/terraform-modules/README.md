# Catalog Terraform modules

Register these directories using the product Git module inspection flow and an
immutable commit. They are source modules, not automatically installed runtime
capabilities or additions to the embedded release/provider mirror.

- `persistent-storage`: namespace-local PVC, stable executor instance name,
  delayed binding supported (`wait_until_bound=false`), outputs claimName/namespace.
- `mysql`: reference implementation for an image supporting MYSQL_DATABASE,
  MYSQL_USER, MYSQL_PASSWORD and MYSQL_ROOT_PASSWORD; accepts existing approved
  Secret name/key refs and never reads their values. Image digest is required.
  Custom images must supply their own compatible module; no Passbolt-specific
  backup/cleanup CronJobs are embedded. Secret delivery prerequisites must exist
  before this module runs; image compatibility is a deployment input.

Removal of consumers leaves resources UNREFERENCED. Explicit resource deletion
is separate and destructive. No default Terraform destroy is run by registration.
