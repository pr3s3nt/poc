package http

import (
	"errors"
	"log"
	"net/http"
	"regexp"

	"orchestrator/internal/adapters/configmemory"
	"orchestrator/internal/adapters/renderbundles"
	"orchestrator/internal/application/accountdiag"
	appcreate "orchestrator/internal/application/application"
	"orchestrator/internal/application/authentication"
	"orchestrator/internal/application/catalog"
	appconfig "orchestrator/internal/application/configuration"
	connectionapp "orchestrator/internal/application/connection"
	appsvc "orchestrator/internal/application/deployment"
	"orchestrator/internal/application/pending"
	"orchestrator/internal/application/preview"
	"orchestrator/internal/application/resourcecleanup"
	"orchestrator/internal/application/secretstores"
	"orchestrator/internal/application/workloadconfig"
	diagdomain "orchestrator/internal/domain/diagnostics"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/renderbundle"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/planning"
	"orchestrator/internal/planning/score"
	configport "orchestrator/internal/ports/configuration"
	"orchestrator/internal/ports/credentials"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/renderregistry"
)

// Public HTTP error contract (T20 / VI-03). Every error response is
// {"error": <fixed sentence>, "code": <stable code>} plus an optional trusted
// "field" and "params". The sentence is chosen from the code, never from
// err.Error(): cause text can quote submitted values, credentials, provider
// output or catalog content. Clients choose their own wording from the code.
const (
	codeInvalidRequest            = "INVALID_REQUEST"
	codeInvalidID                 = "INVALID_ID"
	codeInvalidVariable           = "INVALID_VARIABLE"
	codeInvalidScore              = "INVALID_SCORE"
	codePlanningRejected          = "PLANNING_REJECTED"
	codeDuplicate                 = "DUPLICATE"
	codeUnauthorized              = "UNAUTHORIZED"
	codeInvalidCredentials        = "INVALID_CREDENTIALS"
	codeForbidden                 = "FORBIDDEN"
	codeNotFound                  = "NOT_FOUND"
	codeRequestTooLarge           = "REQUEST_TOO_LARGE"
	codeStaleVersion              = "STALE_VERSION"
	codeStalePreview              = "STALE_PREVIEW"
	codeEnvironmentBusy           = "ENVIRONMENT_BUSY"
	codeConnectionUnavailable     = "CONNECTION_UNAVAILABLE"
	codeAccountTargetMismatch     = "ACCOUNT_TARGET_MISMATCH"
	codeCommitUncertain           = "COMMIT_UNCERTAIN"
	codeVerificationFailed        = "VERIFICATION_FAILED"
	codeVerifyContextMissing      = "VERIFICATION_CONTEXT_MISSING"
	codeVerifyUnreachable         = "VERIFICATION_UNREACHABLE"
	codeVerifyPermissionDenied    = "VERIFICATION_PERMISSION_DENIED"
	codeVerifyCredentialRejected  = "VERIFICATION_CREDENTIAL_REJECTED"
	codeVerifyRegionUnavailable   = "VERIFICATION_REGION_UNAVAILABLE"
	codeVerifyCapabilityUnproven  = "VERIFICATION_CAPABILITY_UNVERIFIABLE"
	codeVerifyPrincipalRefused    = "VERIFICATION_PRINCIPAL_UNSUPPORTED"
	codeKubeconfigInvalid         = "KUBECONFIG_INVALID"
	codeKubeconfigNoContext       = "KUBECONFIG_NO_VALID_CONTEXT"
	codeKubeconfigContextRequired = "KUBECONFIG_CONTEXT_REQUIRED"
	codeKubeconfigContextNotFound = "KUBECONFIG_CONTEXT_NOT_FOUND"
	codeKubeconfigUnsupportedAuth = "KUBECONFIG_UNSUPPORTED_AUTH"
	codeKubeconfigUnsupportedSet  = "KUBECONFIG_UNSUPPORTED_CLUSTER"
	codeKubeconfigInvalidServer   = "KUBECONFIG_INVALID_SERVER"
	codeKubeconfigInvalidTLS      = "KUBECONFIG_INVALID_TLS"
	codeSecretStoreRequired       = "SECRET_STORE_REQUIRED"
	codeSecretStoreUnavailable    = "SECRET_STORE_UNAVAILABLE"
	codeSecretCopyFailed          = "SECRET_COPY_FAILED"
	codeRefRewriteUnsafe          = "REFERENCE_REWRITE_UNSAFE"
	codeRecoveryConfirmRequired   = "RECOVERY_CONFIRMATION_REQUIRED"
	codeRecoveryIncomplete        = "RECOVERY_INCOMPLETE"
	codeCleanupRefused            = "CLEANUP_REFUSED"
	codeConfirmationMismatch      = "RESOURCE_CONFIRMATION_MISMATCH"
	codeOperationLost             = "OPERATION_LOST"
	codeRouteReconcileFailed      = "ROUTE_RECONCILE_FAILED"
	codeServiceUnavailable        = "SERVICE_UNAVAILABLE"
	codeDiagnosticScope           = "DIAGNOSTIC_SCOPE_INVALID"
	codeDiagnosticBusy            = "BUSY"
	codeInternal                  = "INTERNAL_ERROR"
	// Git render bundles (ADR-021, contract RB-C07).
	codeRenderSourceInvalid       = "RENDER_SOURCE_INVALID"
	codeRenderBundleInvalid       = "RENDER_BUNDLE_INVALID"
	codeRenderInputInvalid        = "RENDER_INPUT_INVALID"
	codeRenderSourceUnavailable   = "RENDER_SOURCE_UNAVAILABLE"
	codeRenderBundleUnavailable   = "RENDER_BUNDLE_UNAVAILABLE"
	codeRenderBundleDrift         = "RENDER_BUNDLE_DRIFT"
	codeRenderContractUnsupported = "RENDER_CONTRACT_UNSUPPORTED"
	codeRenderFailed              = "RENDER_FAILED"
)

type publicError struct {
	status  int
	message string
}

var publicErrors = map[string]publicError{
	codeInvalidRequest:            {http.StatusBadRequest, "the request is not valid"},
	codeInvalidID:                 {http.StatusBadRequest, "the id does not follow the id rules"},
	codeInvalidVariable:           {http.StatusBadRequest, "a variable name, value or reference is not valid"},
	codeInvalidScore:              {http.StatusBadRequest, "the Score or its resource parameters are not valid"},
	codePlanningRejected:          {http.StatusBadRequest, "the change cannot be planned"},
	codeDuplicate:                 {http.StatusConflict, "an item with this identity already exists"},
	codeUnauthorized:              {http.StatusUnauthorized, "sign-in is required"},
	codeInvalidCredentials:        {http.StatusUnauthorized, "sign-in failed"},
	codeForbidden:                 {http.StatusForbidden, "access is not allowed"},
	codeNotFound:                  {http.StatusNotFound, "not found"},
	codeRequestTooLarge:           {http.StatusRequestEntityTooLarge, "the request is too large"},
	codeStaleVersion:              {http.StatusConflict, "the data changed since it was loaded"},
	codeStalePreview:              {http.StatusConflict, "the preview is stale"},
	codeEnvironmentBusy:           {http.StatusConflict, "the environment is busy with another operation"},
	codeConnectionUnavailable:     {http.StatusUnprocessableEntity, "the connection is not available"},
	codeAccountTargetMismatch:     {http.StatusUnprocessableEntity, "the credential does not belong to this account's target; register a new account for another target"},
	codeCommitUncertain:           {http.StatusServiceUnavailable, "the result is not known; reload before trying again"},
	codeVerificationFailed:        {http.StatusUnprocessableEntity, "verification failed"},
	codeVerifyContextMissing:      {http.StatusUnprocessableEntity, "verification failed: the context is not configured on the backend host"},
	codeVerifyUnreachable:         {http.StatusUnprocessableEntity, "verification failed: the endpoint could not be reached"},
	codeVerifyPermissionDenied:    {http.StatusUnprocessableEntity, "verification failed: a required permission is missing"},
	codeVerifyCredentialRejected:  {http.StatusUnprocessableEntity, "verification failed: the credential was rejected"},
	codeVerifyRegionUnavailable:   {http.StatusUnprocessableEntity, "verification failed: an allowed region is not usable with this credential"},
	codeVerifyCapabilityUnproven:  {http.StatusUnprocessableEntity, "verification failed: the provisioning permissions could not be verified"},
	codeVerifyPrincipalRefused:    {http.StatusUnprocessableEntity, "verification failed: this kind of credential is not supported"},
	codeKubeconfigInvalid:         {http.StatusBadRequest, "the kubeconfig is not valid"},
	codeKubeconfigNoContext:       {http.StatusBadRequest, "the kubeconfig has no usable context"},
	codeKubeconfigContextRequired: {http.StatusBadRequest, "a kubeconfig context must be selected"},
	codeKubeconfigContextNotFound: {http.StatusBadRequest, "the selected kubeconfig context was not found"},
	codeKubeconfigUnsupportedAuth: {http.StatusBadRequest, "the kubeconfig authentication method is not supported"},
	codeKubeconfigUnsupportedSet:  {http.StatusBadRequest, "the kubeconfig cluster setting is not supported"},
	codeKubeconfigInvalidServer:   {http.StatusBadRequest, "the kubeconfig server address is not valid"},
	codeKubeconfigInvalidTLS:      {http.StatusBadRequest, "the kubeconfig TLS data is not valid"},
	codeSecretStoreRequired:       {http.StatusUnprocessableEntity, "a secret store must be selected"},
	codeSecretStoreUnavailable:    {http.StatusUnprocessableEntity, "the selected secret store is not available"},
	codeSecretCopyFailed:          {http.StatusBadGateway, "secrets could not be copied to the new store"},
	codeRefRewriteUnsafe:          {http.StatusUnprocessableEntity, "the references cannot be updated without losing configuration"},
	codeRecoveryConfirmRequired:   {http.StatusUnprocessableEntity, "recovery needs confirmation that the prior execution stopped"},
	codeRecoveryIncomplete:        {http.StatusConflict, "recovery remains incomplete; the Environment is held"},
	codeCleanupRefused:            {http.StatusConflict, "the cleanup is refused"},
	codeConfirmationMismatch:      {http.StatusBadRequest, "the confirmation name does not match"},
	codeOperationLost:             {http.StatusConflict, "the operation was interrupted"},
	codeRouteReconcileFailed:      {http.StatusBadGateway, "workloads were applied but public routes could not be reconciled"},
	codeServiceUnavailable:        {http.StatusServiceUnavailable, "a required service is not available"},
	codeDiagnosticScope:           {http.StatusUnprocessableEntity, "the diagnostic scope is not valid"},
	codeDiagnosticBusy:            {http.StatusConflict, "this account already has an active diagnostic run"},
	codeInternal:                  {http.StatusInternalServerError, "the request failed"},
	codeRenderSourceInvalid:       {http.StatusBadRequest, "the Git source URL, directory or reference is not valid"},
	codeRenderBundleInvalid:       {http.StatusBadRequest, "the render bundle is not valid"},
	codeRenderInputInvalid:        {http.StatusBadRequest, "the render inputs do not match the bundle schema"},
	codeRenderSourceUnavailable:   {http.StatusServiceUnavailable, "the Git source could not be reached"},
	codeRenderBundleUnavailable:   {http.StatusServiceUnavailable, "the render bundle is not available on this platform"},
	codeRenderBundleDrift:         {http.StatusConflict, "the render bundle content differs from its recorded pin"},
	codeRenderContractUnsupported: {http.StatusBadRequest, "the render bundle contract version is not supported"},
	codeRenderFailed:              {http.StatusUnprocessableEntity, "the render bundle could not render the workload"},
}

// Trusted response fields. A field is echoed only when it is in this set:
// request keys, aliases, names and values never are.
var trustedFields = map[string]bool{
	"name": true, "subdomain": true, "connectionKey": true, "expectedVersion": true, "expectedConfigVersion": true,
	"secretStoreKey": true, "destinationKey": true, "acknowledgeDowntime": true, "priorExecutionStopped": true,
	"resources": true, "status": true, "scoreBefore": true, "scoreAfter": true, "version": true, "token": true, "kubeconfig": true,
	"type.key": true, "type.schema": true, "definition.schema": true, "definition.driver": true, "definition.references": true,
	"definition.key": true, "definition.name": true, "definition.resourceType": true, "definition.version": true,
	"definition.driverType": true, "definition.criteria": true, "definition.driverAccount": true,
	"definition.driverInputs.values": true, "definition.driverInputs.values.source": true, "definition.driverInputs.values.variables.region": true,
	"definition.driverInputs.values.provider_bindings":       true,
	"definition.driverInputs.values.variables.render_bundle": true, "definition.driverInputs.values.variables.render_inputs": true,
	"context.applicationId": true, "context.environmentId": true, "context.resourceId": true, "context.class": true,
	"service.publicPort": true, "confirmationName": true, "selectedResourceIds": true,
	"accessKeyId": true, "secretAccessKey": true, "allowedRegions": true, "verificationRegions": true,
	// UC-17 effectiveness query and baseline fields.
	"from": true, "to": true, "workload": true, "metric": true, "statistic": true, "valueSeconds": true, "source": true,
	"effectiveFrom": true, "sampleSize": true, "measuredFrom": true, "measuredTo": true,
	// Diagnostic scope fields (ADR-022).
	"purpose": true, "mode": true, "region": true, "useCases": true, "application": true, "environment": true,
}

// safeBundleFile bounds a bundle-relative file name echoed as a param.
var safeBundleFile = regexp.MustCompile(`^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+){0,7}$`)

// technicalID bounds a registered resource Type key echoed as a param: lowercase
// letters, digits and hyphens, at most 63 characters (the public ID rule).
var technicalID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// Planner enums allowed as params; the server defines them.
var (
	trustedStages = map[string]bool{
		string(planning.StageScore): true, string(planning.StageBefore): true, string(planning.StageCandidate): true,
		string(planning.StageRoutes): true, string(planning.StageServiceRefs): true, string(planning.StageCatalog): true,
		string(planning.StageGraph): true, string(planning.StageReference): true, string(planning.StageContract): true,
		string(planning.StageSchedule): true,
	}
	trustedReasons = map[string]bool{
		planning.ReasonWorkloadExists: true, planning.ReasonWorkloadNotCurrent: true, planning.ReasonBeforeMismatch: true,
		planning.ReasonUnconfigured:     true,
		planning.ReasonStorageImmutable: true, planning.ReasonRendererNoMatch: true, planning.ReasonRendererAmbiguous: true, planning.ReasonRendererUnavailable: true,
		planning.ReasonPlacement: true, planning.ReasonResourceConflict: true, planning.ReasonResourceNoMatch: true, planning.ReasonResourceAmbiguous: true, planning.ReasonMultiCluster: true, planning.ReasonTargetChanged: true,
		// Allowlisted cleanup refusal reasons (OC-19).
		resourcecleanup.ReasonNotUnreferenced: true, resourcecleanup.ReasonMetadataSyncPending: true, resourcecleanup.ReasonHasConsumers: true, resourcecleanup.ReasonPlatformResource: true,
		resourcecleanup.ReasonOwnershipUnproven: true, resourcecleanup.ReasonTargetUnavailable: true, resourcecleanup.ReasonEnvironmentBusy: true,
	}
)

// failure is the classified public form of one error.
type failure struct {
	status int // optional override that preserves a pre-contract status
	code   string
	field  string
	params map[string]string
	extra  map[string]any
}

// extraKeys are the only additional response keys: the visible operation owner
// and the recovery result DTO. They never replace code, error, field or params.
var extraKeys = []string{"operation", "recovery"}

// safeParams keeps only allow-listed planner stage/reason values.
func safeParams(in map[string]string) map[string]string {
	out := map[string]string{}
	if v, ok := in["resourceType"]; ok && technicalID.MatchString(v) {
		out["resourceType"] = v
	}
	if v, ok := in["stage"]; ok && trustedStages[v] {
		out["stage"] = v
	}
	if v, ok := in["reason"]; ok && trustedReasons[v] {
		out["reason"] = v
	}
	if v, ok := in["file"]; ok && safeBundleFile.MatchString(v) && len(v) <= 256 {
		out["file"] = v
	}
	return out
}

func plannerParams(stage planning.Stage, reason string) map[string]string {
	params := map[string]string{}
	if trustedStages[string(stage)] {
		params["stage"] = string(stage)
	}
	if trustedReasons[reason] {
		params["reason"] = reason
	}
	if len(params) == 0 {
		return nil
	}
	return params
}

// writeCode answers with a fixed envelope for code and an optional trusted field.
func writeCode(w http.ResponseWriter, code, field string) {
	writeFailure(w, failure{code: code, field: field})
}

func writeFailure(w http.ResponseWriter, f failure) {
	spec, ok := publicErrors[f.code]
	if !ok {
		f.code, spec = codeInternal, publicErrors[codeInternal]
	}
	if f.status != 0 {
		spec.status = f.status
	}
	body := map[string]any{"error": spec.message, "code": f.code}
	if trustedFields[f.field] {
		body["field"] = f.field
	}
	if params := safeParams(f.params); len(params) > 0 {
		body["params"] = params
	}
	for _, key := range extraKeys {
		if value, ok := f.extra[key]; ok {
			body[key] = value
		}
	}
	writeJSON(w, spec.status, body)
}

// kubeconfigCodes maps each typed kubeconfig sentinel to its code.
var kubeconfigCodes = []struct {
	err  error
	code string
}{
	{connectionapp.ErrKubeconfigInvalid, codeKubeconfigInvalid},
	{connectionapp.ErrNoValidContext, codeKubeconfigNoContext},
	{connectionapp.ErrContextRequired, codeKubeconfigContextRequired},
	{connectionapp.ErrContextNotFound, codeKubeconfigContextNotFound},
	{connectionapp.ErrUnsupportedAuthentication, codeKubeconfigUnsupportedAuth},
	{connectionapp.ErrUnsupportedClusterSetting, codeKubeconfigUnsupportedSet},
	{connectionapp.ErrInvalidServer, codeKubeconfigInvalidServer},
	{connectionapp.ErrInvalidTLS, codeKubeconfigInvalidTLS},
}

var verificationCodes = []struct {
	err  error
	code string
}{
	{connectionapp.ErrContextMissing, codeVerifyContextMissing},
	{connectionapp.ErrPermissionDenied, codeVerifyPermissionDenied},
	{connectionapp.ErrAuthentication, codeVerifyCredentialRejected},
	{connectionapp.ErrClusterUnreachable, codeVerifyUnreachable},
	{connectionapp.ErrRegionUnavailable, codeVerifyRegionUnavailable},
	{connectionapp.ErrCapabilityUnverifiable, codeVerifyCapabilityUnproven},
	{connectionapp.ErrUnsupportedPrincipal, codeVerifyPrincipalRefused},
}

// classify maps typed errors to a public failure. It never reads err.Error().
// An error without a typed class fails closed as INTERNAL_ERROR (ok == false).
func classify(err error) (failure, bool) {
	var (
		fieldErr   *appcreate.FieldError
		previewErr *catalog.PreviewInvalidError
		catalogErr *catalog.FieldError
		published  *preview.PublicError
		stageErr   *planning.StageError
		paramErr   *workloadconfig.ParamError
	)
	var refused *resourcecleanup.RefusedError
	var cleanupField *resourcecleanup.FieldError
	var renderErr *catalog.RenderError
	var bundleField *renderbundles.BundleInvalidError
	var diagField *diagdomain.FieldError
	switch {
	case errors.As(err, &diagField):
		return failure{code: codeDiagnosticScope, field: diagField.Field}, true
	case errors.Is(err, diagdomain.ErrInvalidScope):
		return failure{code: codeDiagnosticScope}, true
	case errors.Is(err, diagdomain.ErrBusy):
		return failure{code: codeDiagnosticBusy}, true
	case errors.Is(err, accountdiag.ErrUnavailable):
		return failure{code: codeServiceUnavailable}, true
	case errors.Is(err, renderregistry.ErrSourceInvalid):
		return failure{code: codeRenderSourceInvalid}, true
	case errors.Is(err, renderregistry.ErrContractUnsupported), errors.Is(err, renderbundle.ErrUnsupported):
		return failure{code: codeRenderContractUnsupported}, true
	case errors.Is(err, renderregistry.ErrBundleInvalid):
		f := failure{code: codeRenderBundleInvalid}
		if errors.As(err, &bundleField) && safeBundleFile.MatchString(bundleField.File) {
			f.params = map[string]string{"file": bundleField.File}
		}
		return f, true
	case errors.Is(err, renderbundle.ErrInputInvalid), errors.Is(err, resource.ErrRenderInputsInvalid):
		f := failure{code: codeRenderInputInvalid, field: "definition.driverInputs.values.variables.render_inputs"}
		return f, true
	case errors.Is(err, renderregistry.ErrSourceUnavailable):
		return failure{code: codeRenderSourceUnavailable}, true
	case errors.Is(err, renderregistry.ErrBundleUnavailable):
		f := failure{code: codeRenderBundleUnavailable}
		if errors.As(err, &renderErr) {
			f.field = renderErr.Field
		}
		return f, true
	case errors.Is(err, renderregistry.ErrDrift):
		return failure{code: codeRenderBundleDrift}, true
	case errors.Is(err, renderregistry.ErrRenderFailed):
		return failure{code: codeRenderFailed}, true
	case errors.As(err, &refused):
		return failure{code: codeCleanupRefused, params: map[string]string{"reason": refused.RefusalReason()}}, true
	case errors.As(err, &cleanupField):
		return failure{code: codeInvalidRequest, field: cleanupField.Field}, true
	case errors.Is(err, resourcecleanup.ErrConfirmationMismatch):
		return failure{code: codeConfirmationMismatch, field: "confirmationName"}, true
	case errors.Is(err, resourcecleanup.ErrStalePreview):
		return failure{code: codeStalePreview}, true
	case errors.Is(err, resourcecleanup.ErrForbidden):
		return failure{code: codeForbidden}, true
	case errors.Is(err, resourcecleanup.ErrUnavailable):
		return failure{code: codeServiceUnavailable}, true
	case errors.Is(err, resourcecleanup.ErrInvalid):
		return failure{code: codeInvalidRequest}, true
	case errors.Is(err, appsvc.ErrTargetChanged):
		return failure{code: codePlanningRejected, params: map[string]string{"reason": planning.ReasonTargetChanged}}, true
	case errors.Is(err, persistence.ErrEnvironmentBusy):
		return failure{code: codeEnvironmentBusy}, true
	case errors.Is(err, persistence.ErrVersionConflict), errors.Is(err, catalog.ErrVersionConflict), errors.Is(err, appcreate.ErrStaleVersion):
		return failure{code: codeStaleVersion}, true
	case errors.Is(err, pending.ErrStalePreview):
		return failure{code: codeStalePreview}, true
	case errors.Is(err, appconfig.ErrNoStore):
		return failure{code: codeSecretStoreRequired, field: "secretStoreKey"}, true
	case errors.Is(err, configport.ErrStoreUnavailable):
		return failure{code: codeSecretStoreUnavailable, field: "secretStoreKey"}, true
	case errors.Is(err, appconfig.ErrReferenceRewriteUnsafe):
		return failure{code: codeRefRewriteUnsafe}, true
	case errors.Is(err, appconfig.ErrStoreCopy):
		return failure{code: codeSecretCopyFailed}, true
	case errors.Is(err, persistence.ErrOperationLost):
		return failure{code: codeOperationLost}, true
	case errors.Is(err, pending.ErrRouteReconcile):
		return failure{code: codeRouteReconcileFailed}, true
	case errors.Is(err, persistence.ErrNotFound), errors.Is(err, catalog.ErrPreviewNotFound), errors.Is(err, catalog.ErrNotFound):
		return failure{code: codeNotFound}, true
	case errors.Is(err, authentication.ErrInvalidCredentials):
		return failure{code: codeInvalidCredentials}, true
	case errors.Is(err, authentication.ErrUnauthorized):
		return failure{code: codeUnauthorized}, true
	case errors.Is(err, connectionapp.ErrOutcomeUncertain), errors.Is(err, catalog.ErrOutcomeUncertain):
		return failure{code: codeCommitUncertain}, true
	case errors.Is(err, connectionapp.ErrAccountIncompatible):
		return failure{code: codeAccountTargetMismatch}, true
	case errors.Is(err, connectionapp.ErrAccountCredential):
		return failure{code: codeConnectionUnavailable}, true
	case errors.Is(err, connectionapp.ErrKubeconfigTooLarge):
		return failure{code: codeRequestTooLarge}, true
	case errors.Is(err, connectionapp.ErrCredentialStore), errors.Is(err, secretstores.ErrCredentialStore),
		errors.Is(err, credentials.ErrUnavailable), errors.Is(err, configmemory.ErrUnavailable):
		return failure{code: codeServiceUnavailable}, true
	case errors.Is(err, persistence.ErrRecoveryUnconfirmed):
		return failure{code: codeRecoveryConfirmRequired, field: "priorExecutionStopped"}, true
	case errors.Is(err, persistence.ErrDuplicate), errors.Is(err, catalog.ErrDuplicate), errors.Is(err, connectionapp.ErrDuplicate), errors.Is(err, appcreate.ErrDuplicate),
		errors.Is(err, appconfig.ErrConflict):
		f := failure{code: codeDuplicate}
		if errors.As(err, &fieldErr) {
			f.field = fieldErr.Field
		}
		return f, true
	case errors.As(err, &published):
		f := failure{params: plannerParams(published.Stage, published.Reason)}
		switch {
		case errors.Is(err, preview.ErrInvalidScore):
			f.code = codeInvalidScore
		case errors.Is(err, preview.ErrPlanningRejected):
			f.code = codePlanningRejected
		default:
			f.code = codeInvalidRequest
		}
		return f, true
	case errors.As(err, &stageErr):
		return failure{code: codePlanningRejected, params: plannerParams(stageErr.Stage, stageErr.Reason)}, true
	case errors.Is(err, catalog.ErrInvalidID):
		return failure{code: codeInvalidID, field: idField(err)}, true
	case errors.Is(err, catalog.ErrInvalid):
		f := failure{code: codeInvalidRequest}
		if errors.As(err, &previewErr) {
			f.field = previewErr.Field
		} else if errors.As(err, &catalogErr) {
			f.field = catalogErr.Field
		}
		return f, true
	case errors.Is(err, connectionapp.ErrVerification):
		for _, candidate := range verificationCodes {
			if errors.Is(err, candidate.err) {
				return failure{code: candidate.code}, true
			}
		}
		return failure{code: codeVerificationFailed}, true
	case errors.Is(err, secretstores.ErrVerification):
		return failure{code: codeVerificationFailed}, true
	case errors.Is(err, connectionapp.ErrInvalid):
		var connectionField *connectionapp.FieldError
		if errors.As(err, &connectionField) {
			return failure{code: codeInvalidRequest, field: connectionField.Field}, true
		}
		for _, candidate := range kubeconfigCodes {
			if errors.Is(err, candidate.err) {
				return failure{code: candidate.code}, true
			}
		}
		return failure{code: codeInvalidRequest}, true
	case errors.Is(err, appcreate.ErrInvalid):
		f := failure{code: codeInvalidRequest}
		if errors.As(err, &fieldErr) {
			f.field = fieldErr.Field
		}
		return f, true
	case errors.As(err, &paramErr):
		f := failure{code: codeInvalidScore}
		if technicalID.MatchString(paramErr.Type) {
			f.params = map[string]string{"resourceType": paramErr.Type}
		}
		return f, true
	case errors.Is(err, workloadconfig.ErrUnknownResourceType):
		return failure{code: codeInvalidScore, field: "resources"}, true
	case errors.Is(err, workloadconfig.ErrInvalidVariable), errors.Is(err, appconfig.ErrInvalid):
		return failure{code: codeInvalidVariable}, true
	case errors.Is(err, workloadconfig.ErrInvalidRequest), errors.Is(err, secretstores.ErrInvalid), errors.Is(err, pending.ErrInvalid),
		errors.Is(err, appsvc.ErrInvalidStatus):
		return failure{code: codeInvalidRequest}, true
	case errors.Is(err, environment.ErrPublicPortRemoved):
		return failure{code: codeInvalidScore, field: "service.publicPort"}, true
	case errors.Is(err, workloadconfig.ErrInvalid), errors.Is(err, score.ErrInvalid):
		return failure{code: codeInvalidScore}, true
	}
	return failure{}, false
}

// writeError answers any application error with the public contract. A busy
// Environment also carries its visible operation owner. Unclassified errors,
// and errors that only hold cleanup references, are a fixed 500 whose cause
// is neither returned nor logged.
func (s *Server) writeError(w http.ResponseWriter, err error) {
	f, ok := classify(err)
	if !ok {
		var connectionCleanup *connectionapp.CleanupError
		var storeCleanup *secretstores.CleanupError
		switch {
		case errors.As(err, &connectionCleanup):
			log.Printf("request failed: credential cleanup incomplete (reference %s)", connectionCleanup.Reference)
		case errors.As(err, &storeCleanup):
			log.Printf("request failed: credential cleanup incomplete (reference %s)", storeCleanup.Reference)
		default:
			log.Printf("request failed: unclassified internal error (%T)", err)
		}
		writeCode(w, codeInternal, "")
		return
	}
	if f.code == codeEnvironmentBusy {
		f.extra = s.busyExtra(err)
	}
	writeFailure(w, f)
}

// idField names the fixed ID field of a catalog registration failure.
func idField(err error) string {
	var fieldErr *catalog.FieldError
	if errors.As(err, &fieldErr) {
		return fieldErr.Field
	}
	return ""
}
