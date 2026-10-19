package main

import (
	"bytes"
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	withVersion := func(v string) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Version: v}}
	}
	tests := []struct {
		name    string
		ldflags string
		info    *debug.BuildInfo
		ok      bool
		want    string
	}{
		{"ldflags set wins over build info", "v9.9.9", withVersion("v0.1.0"), true, "v9.9.9"},
		{"ldflags set without build info", "v9.9.9", nil, false, "v9.9.9"},
		{"build info module version", "dev", withVersion("v0.1.0"), true, "v0.1.0"},
		{"build info pseudo-version", "dev", withVersion("v0.0.0-20261018142501-abcdef123456"), true, "v0.0.0-20261018142501-abcdef123456"},
		{"devel build stays dev", "dev", withVersion("(devel)"), true, "dev"},
		{"empty module version stays dev", "dev", withVersion(""), true, "dev"},
		{"no build info stays dev", "dev", nil, false, "dev"},
		{"build info not ok stays dev", "dev", withVersion("v0.1.0"), false, "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveVersion(tt.ldflags, tt.info, tt.ok); got != tt.want {
				t.Errorf("resolveVersion(%q) = %q, want %q", tt.ldflags, got, tt.want)
			}
		})
	}
}

func TestRunVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"version"}, &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut.String())
	}
	if got := out.String(); got != "dev\n" {
		t.Errorf("stdout = %q, want %q", got, "dev\n")
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"nope"}, &out, &errOut); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}
