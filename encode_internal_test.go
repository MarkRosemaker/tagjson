package tagjson

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"reflect"
	"strings"
	"testing"
	"time"
)

// These fixtures are local to the test, deliberately decoupled from
// golangci-lint's actual schema: they exercise the encoder's mechanics
// (tags, squash, dashes, duration, zero-omission) in isolation, so a test
// here does not need to change just because golangci-lint restructures a
// linter's settings.

type FixtureBase struct {
	Kept string `mapstructure:"base-kept"`
}

type fixtureNested struct {
	Value int `mapstructure:"value"`
}

type fixture struct {
	Any         any               `mapstructure:"any"`
	NestedPtr   *fixtureNested    `mapstructure:"nested-ptr"`
	Values      map[string]string `mapstructure:"values"`
	Name        string            `mapstructure:"name"`
	unexported  string
	FixtureBase `mapstructure:",squash"`
	Ignored     string `mapstructure:"-"`
	NoTag       string
	Tags        []string      `mapstructure:"tags"`
	Count       int           `mapstructure:"count"`
	Timeout     time.Duration `mapstructure:"timeout"`
	Nested      fixtureNested `mapstructure:"nested"`
	Ratio       float64       `mapstructure:"ratio"`
	Enabled     bool          `mapstructure:"enabled"`
}

func TestEncodeUnsupportedKind(t *testing.T) {
	type unsupported struct {
		Ch chan int `mapstructure:"ch"`
	}

	if _, err := marshalAny(unsupported{Ch: make(chan int)}); err == nil {
		t.Fatal("expected an error for an unsupported field kind, got nil")
	}
}

// marshalAny encodes v through Marshaler{}.encodeValue directly — the same
// path MarshalWriteJSON drives, minus the indentation and trailing-newline
// handling — so these tests can assert against compact JSON literals.
func marshalAny(v any, opts ...json.Options) ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := jsontext.NewEncoder(buf)

	if err := NewMarshaler("mapstructure").encodeValue(enc, reflect.ValueOf(v), opts...); err != nil {
		return nil, err
	}

	// The encoder trails a top-level value with a newline; trimmed here so
	// tests can compare against a plain JSON literal.
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// marshal encodes v as JSON via the package's own reflection path (reusing
// encodeValue directly, so the test does not need a config.Config to exercise
// struct/map/slice/duration/squash mechanics in isolation) and fails the test
// on error.
func marshal(t *testing.T, v any, opts ...json.Options) string {
	t.Helper()

	b, err := marshalAny(v, opts...)
	if err != nil {
		t.Fatal(err)
	}

	return string(b)
}

func TestEncode(t *testing.T) {
	t.Run("only set fields are written, in declared order", func(t *testing.T) {
		f := fixture{Name: "gorepo", Enabled: true}

		got := marshal(t, f)
		want := `{"name":"gorepo","enabled":true}`

		if got != want {
			t.Errorf("got  %s\nwant %s", got, want)
		}
	})

	t.Run("zero values are omitted", func(t *testing.T) {
		got := marshal(t, fixture{Name: "x", Count: 0, Ratio: 0, Tags: nil})
		if strings.Contains(got, "count") || strings.Contains(got, "ratio") || strings.Contains(got, "tags") {
			t.Errorf("zero-valued fields leaked into output: %s", got)
		}
	})

	t.Run("an empty, non-nil slice is treated the same as unset", func(t *testing.T) {
		got := marshal(t, fixture{Name: "x", Tags: []string{}})
		if strings.Contains(got, "tags") {
			t.Errorf("empty slice should have been omitted: %s", got)
		}
	})

	t.Run("squashed fields are inlined, not nested", func(t *testing.T) {
		got := marshal(t, fixture{Name: "x", Kept: "yes"})

		if !strings.Contains(got, `"base-kept":"yes"`) {
			t.Errorf("squashed field missing: %s", got)
		}

		if strings.Contains(got, "FixtureBase") || strings.Contains(got, `"FixtureBase"`) {
			t.Errorf("squashed field was nested under its own key: %s", got)
		}
	})

	t.Run("a struct that is empty only because its fields are empty is itself omitted", func(t *testing.T) {
		// Nested has only a zero int field, so the whole "nested" key must
		// disappear -- not appear as nested:{}.
		got := marshal(t, fixture{Name: "x"})
		if strings.Contains(got, "nested") {
			t.Errorf("an empty nested struct should be omitted entirely: %s", got)
		}
	})

	t.Run("a non-empty nested struct is written", func(t *testing.T) {
		got := marshal(t, fixture{Name: "x", Nested: fixtureNested{Value: 3}})
		if !strings.Contains(got, `"nested":{"value":3}`) {
			t.Errorf("got %s", got)
		}
	})

	t.Run("a nil pointer is omitted, a non-nil one is dereferenced", func(t *testing.T) {
		got := marshal(t, fixture{Name: "x", NestedPtr: &fixtureNested{Value: 5}})
		if !strings.Contains(got, `"nested-ptr":{"value":5}`) {
			t.Errorf("got %s", got)
		}

		got = marshal(t, fixture{Name: "x"})
		if strings.Contains(got, "nested-ptr") {
			t.Errorf("nil pointer should have been omitted: %s", got)
		}
	})

	t.Run("map keys are sorted", func(t *testing.T) {
		got := marshal(t, fixture{Name: "x", Values: map[string]string{"z": "1", "a": "2"}})
		if !strings.Contains(got, `"values":{"a":"2","z":"1"}`) {
			t.Errorf("got %s", got)
		}
	})

	t.Run("a field with no mapstructure tag is never written, whatever its value", func(t *testing.T) {
		got := marshal(t, fixture{Name: "x", NoTag: "should not appear"})
		if strings.Contains(got, "should not appear") || strings.Contains(got, "NoTag") {
			t.Errorf("untagged field leaked into output: %s", got)
		}
	})

	t.Run(`a field tagged "-" is never written, whatever its value`, func(t *testing.T) {
		got := marshal(t, fixture{Name: "x", Ignored: "should not appear"})
		if strings.Contains(got, "should not appear") {
			t.Errorf("dashed field leaked into output: %s", got)
		}
	})

	t.Run("a duration is written in its String form", func(t *testing.T) {
		got := marshal(t, fixture{Name: "x", Timeout: 90 * time.Second}, json.JoinOptions(
			json.WithMarshalers(json.MarshalToFunc(func(enc *jsontext.Encoder, d time.Duration) error {
				return enc.WriteToken(jsontext.String(d.String()))
			})),
		))
		if !strings.Contains(got, `"timeout":"1m30s"`) {
			t.Errorf("got %s", got)
		}
	})

	t.Run("an any field passes its concrete value straight through", func(t *testing.T) {
		got := marshal(t, fixture{Name: "x", Any: map[string]any{"z": 1, "a": []any{"x", "y"}}})
		if !strings.Contains(got, `"any":{"a":["x","y"],"z":1}`) {
			t.Errorf("got %s", got)
		}
	})
}

// TestMarshalerZeroValueReadsMapstructure checks that Marshaler{} (Tag left
// empty) behaves exactly like the package-level MarshalJSON, which is
// documented to read "mapstructure".
func TestMarshalerZeroValueReadsMapstructure(t *testing.T) {
	f := fixture{Name: "gorepo", Enabled: true}

	got := marshal(t, f)
	want := `{"name":"gorepo","enabled":true}`

	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}
