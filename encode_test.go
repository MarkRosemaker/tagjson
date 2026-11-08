package tagjson_test

import (
	"encoding/json/jsontext"
	"reflect"
	"strings"
	"testing"
	"time"

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

type Base struct {
	ID string `mapstructure:"id"`
}

type inner struct {
	N int `mapstructure:"n"`
}

type outer struct {
	*Base `mapstructure:",squash"`

	Name    string            `mapstructure:"name,omitempty"`
	Timeout time.Duration     `mapstructure:"timeout"`
	Ints    [3]int            `mapstructure:"ints"`
	List    []any             `mapstructure:"list"`
	ByNum   map[int]string    `mapstructure:"by-num"`
	Inners  map[string]inner  `mapstructure:"inners"`
	Ptr     *inner            `mapstructure:"ptr"`
	Any     any               `mapstructure:"any"`
	Labels  map[string]string `mapstructure:"labels"`
}

func TestMarshal(t *testing.T) {
	m := tagjson.NewMarshaler("mapstructure")

	for _, tc := range []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, `null`},
		{"nil pointer", (*outer)(nil), `null`},
		{"empty struct", outer{}, `{}`},
		{"pointer to struct", &outer{Name: "x"}, `{"name":"x"}`},
		{"tag options after the comma are not part of the key", outer{Name: "x"}, `{"name":"x"}`},
		{"duration in String form", outer{Timeout: 90 * time.Minute}, `{"timeout":"1h30m0s"}`},
		{"top-level duration", 2 * time.Second, `"2s"`},
		{"squashed pointer embed is inlined", outer{Base: &Base{ID: "b"}, Name: "x"}, `{"id":"b","name":"x"}`},
		{"squashed embed with only empty fields adds nothing", outer{Base: &Base{}, Name: "x"}, `{"name":"x"}`},
		{"array keeps zero elements", outer{Ints: [3]int{0, 2, 0}}, `{"ints":[0,2,0]}`},
		{"all-zero array is omitted", outer{Ints: [3]int{}}, `{}`},
		{"slice keeps nil and zero elements", outer{List: []any{nil, 0, ""}}, `{"list":[null,0,""]}`},
		{"non-string map keys are stringified and sorted as strings", outer{ByNum: map[int]string{10: "a", 2: "b", 1: "c"}}, `{"by-num":{"1":"c","10":"a","2":"b"}}`},
		{"empty map values are omitted", outer{Labels: map[string]string{"a": "", "b": "x"}}, `{"labels":{"b":"x"}}`},
		{"map values are encoded by tag", outer{Inners: map[string]inner{"k": {N: 1}, "z": {}}}, `{"inners":{"k":{"n":1}}}`},
		{"pointer to empty struct is omitted", outer{Ptr: &inner{}}, `{}`},
		{"interface holding a pointer to a tagged struct reads its tags", outer{Any: &inner{N: 4}}, `{"any":{"n":4}}`},
		{"interface holding a pointer to an empty struct is omitted", outer{Any: &inner{}}, `{}`},
		{"interface holding a duration", outer{Any: time.Minute}, `{"any":"1m0s"}`},
		{"array of tagged structs", [2]inner{{N: 1}, {}}, `[{"n":1},{}]`},
		{"top-level slice of structs", []inner{{N: 1}, {}}, `[{"n":1},{}]`},
		{"top-level map", map[string]any{"b": 1, "a": inner{N: 2}}, `{"a":{"n":2},"b":1}`},
		{"scalar", "plain", `"plain"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := m.Marshal(tc.in)
			if err != nil {
				t.Fatal(err)
			}

			if got := string(b); got != tc.want {
				t.Errorf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestMarshalInlineOptions(t *testing.T) {
	type embedded struct {
		A string `yaml:"a"`
	}

	for _, opt := range []string{"squash", "inline", "embed"} {
		t.Run(opt, func(t *testing.T) {
			v := reflect.New(reflect.StructOf([]reflect.StructField{
				{Name: "E", Type: reflect.TypeFor[embedded](), Tag: reflect.StructTag(`yaml:",` + opt + `"`)},
				{Name: "B", Type: reflect.TypeFor[string](), Tag: `yaml:"b"`},
			})).Elem()
			v.Field(0).Set(reflect.ValueOf(embedded{A: "1"}))
			v.Field(1).SetString("2")

			b, err := tagjson.NewMarshaler("yaml").Marshal(v.Interface())
			if err != nil {
				t.Fatal(err)
			}

			if got, want := string(b), `{"a":"1","b":"2"}`; got != want {
				t.Errorf("got  %s\nwant %s", got, want)
			}
		})
	}
}

func TestMarshalErrors(t *testing.T) {
	type WithChan struct {
		Ch chan int `mapstructure:"ch"`
	}

	type squashed struct {
		WithChan `mapstructure:",squash"`
	}

	type withList struct {
		L []any `mapstructure:"l"`
	}

	m := tagjson.NewMarshaler("mapstructure")

	for _, tc := range []struct {
		name   string
		in     any
		prefix string
	}{
		{"field", WithChan{Ch: make(chan int)}, "tagjson: Ch: "},
		{"squashed field", squashed{WithChan{Ch: make(chan int)}}, "tagjson: WithChan: Ch: "},
		{"slice element", withList{L: []any{1, make(chan int)}}, "tagjson: L: [1]: "},
		{"map value", map[string]any{"k": func() {}}, "tagjson: k: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := m.Marshal(tc.in)
			if err == nil {
				t.Fatalf("expected an error, got %s", b)
			}

			if !strings.HasPrefix(err.Error(), tc.prefix) {
				t.Errorf("error %q does not start with %q", err, tc.prefix)
			}

			if b != nil {
				t.Errorf("expected no output on error, got %s", b)
			}
		})
	}
}
