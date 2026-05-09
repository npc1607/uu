package logtail

import "testing"

func TestLastLines(t *testing.T) {
	content := []byte("one\ntwo\nthree\n")
	if got := string(LastLines(content, 2)); got != "two\nthree" {
		t.Fatalf("LastLines = %q, want %q", got, "two\nthree")
	}
	if got := string(LastLines(content, 10)); got != "one\ntwo\nthree" {
		t.Fatalf("LastLines = %q, want full content", got)
	}
	if got := LastLines(content, 0); got != nil {
		t.Fatalf("LastLines with zero lines = %q, want nil", got)
	}
}
