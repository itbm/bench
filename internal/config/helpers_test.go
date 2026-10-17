package config

import (
	"reflect"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/layout"
)

// requireLayout skips a test while internal/layout is still a stub, because
// the code under test calls layout.Validate.
func requireLayout(t *testing.T) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Skipf("layout.Validate isn't implemented yet (%v)", r)
		}
	}()
	_ = layout.Validate(&layout.Node{Type: layout.TypePane, ID: "p1"})
}

// where is a problem's location, for comparing problems without their
// messages.
type where struct{ Tab, Field string }

func wheres(problems []Problem) []where {
	out := make([]where, len(problems))
	for i, p := range problems {
		out[i] = where{p.Tab, p.Field}
	}
	return out
}

// checkWheres fails unless problems are at exactly the given places, in
// order, each with a message.
func checkWheres(t *testing.T, problems []Problem, want []where) {
	t.Helper()
	if got := wheres(problems); !reflect.DeepEqual(got, want) && !(len(got) == 0 && len(want) == 0) {
		t.Errorf("problems at %v, want %v\nproblems: %s", got, want, describe(problems))
	}
	for _, p := range problems {
		if p.Message == "" {
			t.Errorf("problem %+v has no message", p)
		}
	}
}

func describe(problems []Problem) string {
	parts := make([]string, len(problems))
	for i, p := range problems {
		parts[i] = p.Error()
	}
	return strings.Join(parts, "\n          ")
}

// mustParse parses a config that must have no problems.
func mustParse(t *testing.T, doc string) *Config {
	t.Helper()
	cfg, problems, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(problems) > 0 {
		t.Fatalf("Parse: unexpected problems:\n%s\nin:\n%s", describe(problems), doc)
	}
	return cfg
}

// oneTab is a minimal valid tab, for documents testing other things.
const oneTab = `
[[tab]]
name = "Code"
layout = "single"
`
