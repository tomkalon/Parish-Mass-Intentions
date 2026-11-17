package main

import (
	"regexp"
	"testing"
)

func TestAppVersion(t *testing.T) {
	data := []byte(`{"RT_VERSION": {"#1": {"0000": {"info": {"0415": {"ProductVersion": "2.3.4"}}}}}}`)
	if got := appVersion(data); got != "2.3.4" {
		t.Errorf("appVersion = %q, want 2.3.4", got)
	}

	for _, broken := range []string{"", "{}", "nie json", `{"RT_VERSION": {"#1": {"0000": {"info": {}}}}}`} {
		if got := appVersion([]byte(broken)); got != "" {
			t.Errorf("appVersion(%q) = %q, want empty", broken, got)
		}
	}
}

func TestEmbeddedVersionMatchesWindowsResources(t *testing.T) {
	got := appVersion(winres)
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(got) {
		t.Errorf("version from build/windows/winres.json = %q, want X.Y.Z", got)
	}
}
