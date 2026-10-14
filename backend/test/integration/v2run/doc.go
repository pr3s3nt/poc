// Package v2run is the test-support library of the real Terraform v2
// acceptance runner (M04 I04-10): private configuration and run manifest,
// the independent cleanup supervisor, pod-capacity and cost-estimate gates,
// verification inventories and evidence validation.
//
// It is NOT product code: nothing under backend/internal imports it. The
// runner, supervisor and checks are operator tooling for scenarios whose
// business steps all run through the Web Console. Host credentials used here
// (preflight, supervisor safety net) never reach the product, which resolves
// scoped Connections only.
package v2run
