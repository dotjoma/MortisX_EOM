package version

import "testing"

func TestVersionStrings(t *testing.T) {
	expectedStr := String()
	if expectedStr == "" {
		t.Fatalf("String() should not be empty")
	}
	expectedTitle := WindowTitle()
	if expectedTitle != AppName+" "+expectedStr {
		t.Fatalf("unexpected WindowTitle: %s", expectedTitle)
	}
	expectedSemver := SemverString()
	if expectedSemver == "" {
		t.Fatalf("SemverString() should not be empty")
	}
}
