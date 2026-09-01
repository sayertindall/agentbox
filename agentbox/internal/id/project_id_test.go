package id

import "testing"

func TestParseProjectIDAcceptsCanonicalValue(t *testing.T) {
	projectID, err := ParseProjectID("example-api-2")
	if err != nil {
		t.Fatalf("ParseProjectID() error = %v", err)
	}
	if got, want := projectID.String(), "example-api-2"; got != want {
		t.Fatalf("ParseProjectID() = %q, want %q", got, want)
	}
}

func TestParseProjectIDRejectsTraversal(t *testing.T) {
	for _, value := range []string{"..", "../project", "project/../other", "project\\other"} {
		if _, err := ParseProjectID(value); err == nil {
			t.Errorf("ParseProjectID(%q) succeeded, want rejection", value)
		}
	}
}

func TestParseProjectIDRejectsUnicode(t *testing.T) {
	for _, value := range []string{"café", "проєкт", "project\u00a0name"} {
		if _, err := ParseProjectID(value); err == nil {
			t.Errorf("ParseProjectID(%q) succeeded, want rejection", value)
		}
	}
}
