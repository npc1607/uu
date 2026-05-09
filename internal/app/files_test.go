package app

import (
	"os"
	"testing"
)

func TestEnsureTrailingSeparator(t *testing.T) {
	path := "/tmp/uu"
	want := path + string(os.PathSeparator)
	if got := ensureTrailingSeparator(path); got != want {
		t.Fatalf("ensureTrailingSeparator(%q) = %q, want %q", path, got, want)
	}
	if got := ensureTrailingSeparator(want); got != want {
		t.Fatalf("ensureTrailingSeparator(%q) = %q, want unchanged", want, got)
	}
}
