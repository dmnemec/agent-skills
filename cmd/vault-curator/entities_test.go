package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/dmnemec/agent-skills/internal/vaultcurator/entities"
)

const fixtureVault = "../../testdata/vault"

func TestEntitiesJSON(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"entities", "--vault", fixtureVault, "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut.String())
	}
	var res entities.Result
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	got := map[string]bool{}
	for _, c := range res.Candidates {
		got[c.Name] = true
	}
	for _, name := range []string{"Teodor Quill", "Sana Okafor-Lind", "Oskar Fenwick", "Lio Haverkamp"} {
		if !got[name] {
			t.Errorf("candidate %q missing from %s", name, out.String())
		}
	}
	if len(res.Entities) == 0 {
		t.Error("no entities listed")
	}
}

func TestEntitiesText(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"entities", "--vault", fixtureVault}, &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("candidate Teodor Quill")) {
		t.Errorf("text output lacks Teodor Quill:\n%s", out.String())
	}
}

func TestEntitiesLinksTo(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"entities", "--vault", fixtureVault, "--links-to", "Mira", "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut.String())
	}
	var res entities.Backlinks
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if res.Target.Path != "people/Mira Vantongeren.md" || len(res.Notes) == 0 {
		t.Errorf("result = %+v", res)
	}
	out.Reset()
	if code := run([]string{"entities", "--vault", fixtureVault, "--links-to", "Nobody"}, &out, &errOut); code != 1 {
		t.Errorf("unknown target: exit code = %d, want 1", code)
	}
}

func TestEntitiesUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"entities", "--min-count", "0"}, &out, &errOut); code != 2 {
		t.Errorf("--min-count 0: exit code = %d, want 2", code)
	}
	if code := run([]string{"entities", "extra"}, &out, &errOut); code != 2 {
		t.Errorf("extra arg: exit code = %d, want 2", code)
	}
	if code := run([]string{"entities", "--vault", "no/such/dir"}, &out, &errOut); code != 1 {
		t.Errorf("missing vault: exit code = %d, want 1", code)
	}
}
