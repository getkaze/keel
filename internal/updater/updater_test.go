package updater

import (
	"testing"
)

func TestIsNewer_SimpleNewer(t *testing.T) {
	if !IsNewer("0.3.0", "0.2.0") {
		t.Error("0.3.0 should be newer than 0.2.0")
	}
}

func TestIsNewer_MajorVersion(t *testing.T) {
	if !IsNewer("1.0.0", "0.99.99") {
		t.Error("1.0.0 should be newer than 0.99.99")
	}
}

func TestIsNewer_TenVsNine(t *testing.T) {
	if !IsNewer("0.10.0", "0.9.0") {
		t.Error("0.10.0 should be newer than 0.9.0 (semantic, not string)")
	}
}

func TestIsNewer_Equal(t *testing.T) {
	if IsNewer("0.3.0", "0.3.0") {
		t.Error("equal versions should not be 'newer'")
	}
}

func TestIsNewer_OlderVersion(t *testing.T) {
	if IsNewer("0.1.0", "0.2.0") {
		t.Error("0.1.0 should not be newer than 0.2.0")
	}
}

func TestIsNewer_PatchVersion(t *testing.T) {
	if !IsNewer("0.3.1", "0.3.0") {
		t.Error("0.3.1 should be newer than 0.3.0")
	}
}

func TestIsNewer_WithVPrefix(t *testing.T) {
	if !IsNewer("v0.3.0", "v0.2.0") {
		t.Error("v0.3.0 should be newer than v0.2.0 (v prefix stripped)")
	}
}

func TestIsNewer_InvalidFallback(t *testing.T) {
	// Non-semver strings fall back to string comparison (not equal = newer)
	if !IsNewer("dev", "0.3.0") {
		t.Error("non-parseable versions with different strings should return true (fallback)")
	}
}

func TestIsNewer_BothInvalid_Equal(t *testing.T) {
	if IsNewer("dev", "dev") {
		t.Error("same non-parseable strings should return false")
	}
}

func TestParseSemver_Valid(t *testing.T) {
	parts, ok := parseSemver("1.2.3")
	if !ok {
		t.Fatal("expected ok=true for valid semver")
	}
	if parts != [3]int{1, 2, 3} {
		t.Errorf("expected [1,2,3], got %v", parts)
	}
}

func TestParseSemver_WithV(t *testing.T) {
	parts, ok := parseSemver("v0.10.5")
	if !ok {
		t.Fatal("expected ok=true for v-prefixed semver")
	}
	if parts != [3]int{0, 10, 5} {
		t.Errorf("expected [0,10,5], got %v", parts)
	}
}

func TestParseSemver_Invalid(t *testing.T) {
	_, ok := parseSemver("dev")
	if ok {
		t.Error("expected ok=false for non-semver string")
	}
}

func TestParseSemver_TwoParts(t *testing.T) {
	_, ok := parseSemver("1.2")
	if ok {
		t.Error("expected ok=false for two-part version")
	}
}
