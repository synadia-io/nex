package actors

import (
	"testing"
)

func TestUriParser(t *testing.T) {
	tests := []struct {
		uri              string
		expectedSchema   string
		expectedLocation string
		expectedFilePath string
		expectedTag      string
	}{
		// tests latest tags
		{"file:///tmp/foo", SchemeFile, "", "/tmp/foo", ""},
		{"oci://synadia/foo", SchemeOCI, "synadia", "foo", "latest"},
		{"nats://myobject/foo", SchemeNATS, "myobject", "foo", "latest"},
		{"file://C:\\\\my\\\\file\\\\path", SchemeFile, "", "C:\\\\my\\\\file\\\\path", ""},
		// tests with a tag
		{"oci://synadia/foo:derp", SchemeOCI, "synadia", "foo", "derp"},
		{"oci://synadia:8000/foo:derp", SchemeOCI, "synadia:8000", "foo", "derp"},
		{"oci://url.io/repo/derp:latest", SchemeOCI, "url.io", "repo/derp", "latest"},
		{"oci://url.io:10000/repo/derp:latest", SchemeOCI, "url.io:10000", "repo/derp", "latest"},
		{"nats://myobject/foo:derp", SchemeNATS, "myobject", "foo", "derp"},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			result, err := parseUri(tt.uri)
			if err != nil {
				t.Errorf("failed to parse uri: %v", err)
			}
			if result.location != tt.expectedLocation {
				t.Errorf("expected %s, got %s", tt.expectedLocation, result.location)
			}
			if result.schema != tt.expectedSchema {
				t.Errorf("expected %s, got %s", tt.expectedSchema, result.schema)
			}
			if result.path != tt.expectedFilePath {
				t.Errorf("expected %s, got %s", tt.expectedFilePath, result.path)
			}
			if result.tag != tt.expectedTag {
				t.Errorf("expected %s, got %s", tt.expectedTag, result.tag)
			}

		})
	}
}
