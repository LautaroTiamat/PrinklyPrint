//go:build !windows

package insecure

import "testing"

// TestAllowAnyOriginStubFalse: fuera de Windows (dev/CI) el modo inseguro nunca
// está activo — la marca real vive en HKLM, que solo existe en Windows.
func TestAllowAnyOriginStubFalse(t *testing.T) {
	if AllowAnyOrigin() {
		t.Error("AllowAnyOrigin() debería ser false fuera de Windows (stub)")
	}
}
