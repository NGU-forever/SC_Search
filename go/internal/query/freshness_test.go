package query

import "testing"

func TestIsComparableGameVersion(t *testing.T) {
	if !IsComparableGameVersion("4.10.3-live.13220821") {
		t.Fatal("expected comparable")
	}
	if !IsComparableGameVersion("3.24.2") {
		t.Fatal("expected comparable")
	}
	if IsComparableGameVersion("aliases") {
		t.Fatal("aliases must not be comparable")
	}
	if IsComparableGameVersion("localization-full") {
		t.Fatal("localization must not be comparable")
	}
}
