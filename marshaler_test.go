package tagjson_test

import (
	"encoding/json/jsontext"
	"testing"

	"github.com/MarkRosemaker/tagjson"
)

type testStruct struct {
	Foo          string  `mapstructure:"foo"`
	Bar          int     `mapstructure:"bar"`
	DifferentTag float64 `mytag:"bar"`
	NotTagged    bool
	internal     int
}

func TestMarshalWriteJSON(t *testing.T) {
	got, err := tagjson.NewMarshaler("mapstructure").Marshal(testStruct{
		Foo:          "baz",
		Bar:          3,
		DifferentTag: 3.14,
		NotTagged:    true,
		internal:     5,
	}, jsontext.WithIndent("  "))
	if err != nil {
		t.Fatal(err)
	}

	if want := `{
  "foo": "baz",
  "bar": 3
}`; string(got) != want {
		t.Errorf("got=%s, want=%s", got, want)
	}
}
