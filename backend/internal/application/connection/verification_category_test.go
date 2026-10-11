package connection

import (
	"errors"
	"testing"
)

func TestVerificationErrorsKeepCategoryWithoutChangingMessage(t *testing.T) {
	for _, category := range []error{ErrContextMissing, ErrPermissionDenied, ErrClusterUnreachable} {
		err := verificationError(category)
		if !errors.Is(err, ErrVerification) || !errors.Is(err, category) {
			t.Errorf("%v lost its class: %v", category, err)
		}
	}
	for _, category := range []error{ErrPermissionDenied, ErrAuthentication, ErrClusterUnreachable} {
		err := kubeconfigVerificationError(category)
		if !errors.Is(err, ErrVerification) || !errors.Is(err, category) {
			t.Errorf("%v lost its class: %v", category, err)
		}
	}
}
