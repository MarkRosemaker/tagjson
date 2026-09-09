// Package tagjson marshals a struct-tagged Go value to JSON, reading
// whichever struct tag the caller names, rather than requiring a parallel set of "json" tags.
//
// This package only produces JSON, deliberately: JSON is what every YAML
// library already round-trips through, so a caller wanting YAML converts
// [MarshalJSON]'s output with whichever library they prefer, rather than this
// package choosing one for them and every caller paying for it. For example,
// with [github.com/MarkRosemaker/json2yaml] and [gopkg.in/yaml.v3]:
//
//	b, err := tagjson.MarshalJSON(v)
//	node, err := json2yaml.Convert(jsontext.Value(b))
//	yaml.NewEncoder(w).Encode(node)
//
// # Semantics
//
// The mapping follows mapstructure's rules, in reverse:
//
//   - A field tagged "-", or carrying no tag at all, is something decoding
//     would not have populated from a decoded map either, and is omitted.
//   - A field tagged ",squash" is an embedded struct whose own fields are
//     inlined into the enclosing object, rather than nested under a key of
//     their own — mapstructure's flattening, run the other direction.
//   - Every other tag names the field's key verbatim: "tab-len", not
//     "TabLen".
//   - A field left at its zero value — an empty string, an unset bool, a nil
//     or empty slice or map, a struct whose own fields are all likewise empty
//     — is omitted. There is no way, from the tag alone, to know whether a
//     particular zero value is the type's actual default; a caller that
//     needs to distinguish "unset" from "explicitly set to the zero value"
//     for a specific field — because decoding treats them differently —
//     needs a hand-written marshaler for that one type instead.
//   - Map keys are not looked up in any tag — a map is by definition a set of
//     caller-chosen names, not fixed fields — and are written as-is, sorted,
//     since a Go map does not otherwise have a stable order.
//   - [time.Duration] is written as its [time.Duration.String] form
//     ("1h30m"), not as a number of nanoseconds.
//
// [mapstructure]: https://github.com/go-viper/mapstructure
package tagjson

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	"fmt"
	"io"
	"reflect"
)

// Marshaler marshals a Go value whose struct fields are tagged with a given tag,
// treating that tag exactly like the package doc comment's semantics describe.
//
// The zero value reads "mapstructure", the same as the package-level
// functions; set Tag to read any other struct tag name instead — "yaml",
// "toml", a project's own — with identical semantics.
//
// Methods are named Marshal/MarshalWrite, not MarshalJSON/MarshalWriteJSON:
// a method actually named MarshalJSON is what [encoding/json] looks for to
// mean "this type knows how to marshal itself" — the opposite of what
// Marshaler does, which is marshal some *other* value handed to it.
type Marshaler struct {
	// the struct tag name to read.
	tag string
}

func NewMarshaler(tag string) *Marshaler {
	if tag == "" {
		panic("no tag given")
	}

	return &Marshaler{tag: tag}
}

// Marshal encodes v as JSON.
func (m Marshaler) Marshal(v any, opts ...json.Options) ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := m.MarshalWrite(buf, v, opts...); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// MarshalWrite encodes v as JSON, writing it to w.
func (m Marshaler) MarshalWrite(w io.Writer, v any, opts ...json.Options) error {
	// Buffered rather than written to w directly, so the trailing newline
	// jsontext.Encoder leaves after a top-level value — its habit, not this
	// package's — can be trimmed before anything reaches w: Marshal's result
	// should look like encoding/json.Marshal's, not encoding/json's
	// Encoder.Encode.
	buf := &bytes.Buffer{}
	enc := jsontext.NewEncoder(buf, opts...)

	// reflect.ValueOf(&v).Elem(), not reflect.ValueOf(v): boxing a large
	// struct straight into the any that ValueOf takes reproducibly crashed a
	// -race build with "found bad pointer in Go heap" a few call frames
	// deeper — reached through a caller's own JSON-to-YAML conversion chain
	// but not through MarshalJSON called directly at the same depth.
	// Reproduces on go1.27 both with and without GOEXPERIMENT=jsonv2, so it is
	// not the experiment; it reads as a toolchain escape-analysis defect for
	// boxing a value this large at depth, not anything wrong with the value
	// itself. Taking its address first forces an unambiguous heap allocation
	// before reflection ever sees it, and the crash has not reproduced since.
	// See TestMarshalYAML.
	rv := reflect.ValueOf(&v).Elem().Elem()

	if err := m.encodeValue(enc, rv, opts...); err != nil {
		return fmt.Errorf("tagjson: %w", err)
	}

	if _, err := w.Write(bytes.TrimRight(buf.Bytes(), "\n")); err != nil {
		return fmt.Errorf("tagjson: %w", err)
	}

	return nil
}
