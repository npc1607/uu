package downloader

import "testing"

func TestWithOutputText(t *testing.T) {
	tests := map[string]string{
		"http://example.test/api":        "http://example.test/api?output=text",
		"http://example.test/api?a=true": "http://example.test/api?a=true&output=text",
	}
	for input, want := range tests {
		if got := WithOutputText(input); got != want {
			t.Fatalf("WithOutputText(%q) = %q, want %q", input, got, want)
		}
	}
}
