package fleet

import "testing"

func TestRegistrationIdentityMatchesPhysicalTarget(t *testing.T) {
	for _, tc := range []struct {
		name, cluster, clientID, secret, bootstrap, manager, target string
		want                                                        bool
	}{
		{"remote default identity", "remote", "remote-uid", "", "", "manager-uid", "remote-uid", true},
		{"bootstrap local", "local", "", "local-cluster", "fleet-bootstrap", "same-uid", "same-uid", true},
		{"same name different cluster", "local", "", "local-cluster", "fleet-bootstrap", "manager-uid", "other-uid", false},
		{"custom opaque identity", "remote", "custom-id", "", "", "manager-uid", "remote-uid", false},
		{"missing physical identity", "local", "", "local-cluster", "fleet-bootstrap", "", "", false},
		{"unproven local registration", "local", "", "", "", "same-uid", "same-uid", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := registrationMatches(tc.cluster, tc.clientID, tc.secret, tc.bootstrap, tc.manager, tc.target); got != tc.want {
				t.Fatalf("match=%v want=%v", got, tc.want)
			}
		})
	}
}
