package tagjson_test

import (
	"encoding/json/jsontext"
	"strings"
	"testing"

	"github.com/MarkRosemaker/tagjson"
)

// TestMarshalerCustomTag checks that Tag genuinely changes which struct tag
// is read — not just that "mapstructure" happens to work — using "yaml" as a
// stand-in for "any tag name the caller chooses".
func TestMarshalerCustomTag(t *testing.T) {
	type yamlFixture struct {
		Name    string `yaml:"name"`
		Ignored string `yaml:"-"`
		NoTag   string
		// A mapstructure tag here must be irrelevant when Tag is "yaml": it
		// names a key that only matters to a different Marshaler.
		Other string `mapstructure:"other"`
	}

	v := yamlFixture{Name: "gorepo", Ignored: "nope", NoTag: "nope", Other: "nope"}

	m := tagjson.NewMarshaler("yaml")

	b, err := m.Marshal(v, jsontext.WithIndent("  "))
	if err != nil {
		t.Fatal(err)
	}

	got := string(b)

	if !strings.Contains(got, `"name": "gorepo"`) {
		t.Errorf("the yaml-tagged field is missing: %s", got)
	}

	if strings.Contains(got, "nope") {
		t.Errorf("a field not tagged for \"yaml\" leaked into the output: %s", got)
	}
}
