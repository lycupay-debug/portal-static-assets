// auth_api_test.go verifies the registration feature flag defaults to closed.

package main

import (
	"os"
	"testing"
)

// Registration is an explicit deployment capability and remains disabled unless configured.
func TestPortalRegistrationRequiresExplicitEnablement(t *testing.T) {
	original, existed := os.LookupEnv("PERSONAL_PORTAL_REGISTRATION_ENABLED")
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv("PERSONAL_PORTAL_REGISTRATION_ENABLED", original)
		} else {
			_ = os.Unsetenv("PERSONAL_PORTAL_REGISTRATION_ENABLED")
		}
	})

	for _, value := range []string{"", "0", "false", "no", "off", "unexpected"} {
		if err := os.Setenv("PERSONAL_PORTAL_REGISTRATION_ENABLED", value); err != nil {
			t.Fatal(err)
		}
		if portalRegistrationEnabled() {
			t.Errorf("registration enabled for value %q", value)
		}
	}
	for _, value := range []string{"1", "true", "yes", "on"} {
		if err := os.Setenv("PERSONAL_PORTAL_REGISTRATION_ENABLED", value); err != nil {
			t.Fatal(err)
		}
		if !portalRegistrationEnabled() {
			t.Errorf("registration disabled for explicit value %q", value)
		}
	}
}
