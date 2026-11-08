package tagjson_test

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"testing"

	"github.com/MarkRosemaker/tagjson"
)

type testStruct struct {
	Foo          string  `mapstructure:"foo"`
	Bar          int     `mapstructure:"bar"`
	DifferentTag float64 `                   mytag:"bar"`
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

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errWrite }

var errWrite = errors.New("write failed")

func TestMarshalWrite(t *testing.T) {
	m := tagjson.NewMarshaler("mapstructure")

	t.Run("writes compact JSON without a trailing newline", func(t *testing.T) {
		var buf bytes.Buffer
		if err := m.MarshalWrite(&buf, testStruct{Foo: "baz"}); err != nil {
			t.Fatal(err)
		}

		if got, want := buf.String(), `{"foo":"baz"}`; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("a writer error is returned", func(t *testing.T) {
		err := m.MarshalWrite(failingWriter{}, testStruct{Foo: "baz"})
		if !errors.Is(err, errWrite) {
			t.Errorf("got %v, want %v", err, errWrite)
		}
	})

	t.Run("an encoding error writes nothing", func(t *testing.T) {
		var buf bytes.Buffer
		if err := m.MarshalWrite(&buf, map[string]any{"a": 1, "b": make(chan int)}); err == nil {
			t.Fatal("expected an error, got nil")
		}

		if buf.Len() != 0 {
			t.Errorf("partial output reached the writer: %s", buf.String())
		}
	})
}

func TestNewMarshalerPanicsOnEmptyTag(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected a panic for an empty tag")
		}
	}()

	tagjson.NewMarshaler("")
}
