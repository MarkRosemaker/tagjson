// Package mapstructurejson marshals a struct-tagged Go value to JSON, reading
// whichever struct tag the caller names — [mapstructure] by default — rather
// than requiring a parallel set of "json" tags.
//
// [mapstructure] is a decode-only library: it reads a plain map (typically
// decoded from YAML) into a Go struct tagged "mapstructure:\"foo-bar\"", but
// offers nothing for the reverse direction. Marshaling such a struct directly
// with [encoding/json] falls back to its raw Go field names ("FooBar") and
// writes every zero value out explicitly, since encoding/json only ever looks
// at "json" tags. [MarshalJSON] reads the "mapstructure" tag instead, through
// reflection, so any type already set up for mapstructure decoding can be
// marshaled back out with no further annotation. [Marshaler] reads any other
// tag the same way, for a type set up for some other library instead — a
// project's own config-loading tag, [github.com/BurntSushi/toml]'s "toml",
// or anything else that follows the same "-", ",squash", and plain-name
// conventions.
//
// This package only produces JSON, deliberately: JSON is what every YAML
// library already round-trips through, so a caller wanting YAML converts
// [MarshalJSON]'s output with whichever library they prefer, rather than this
// package choosing one for them and every caller paying for it. For example,
// with [github.com/MarkRosemaker/json2yaml] and [gopkg.in/yaml.v3]:
//
//	b, err := mapstructurejson.MarshalJSON(v)
//	node, err := json2yaml.Convert(jsontext.Value(b))
//	yaml.NewEncoder(w).Encode(node)
//
// # Semantics
//
// The mapping follows mapstructure's own rules, in reverse:
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
package mapstructurejson

import (
	"bytes"
	"encoding/json/jsontext"
	"fmt"
	"io"
	"reflect"
)

// defaultTag is the struct tag [Marshaler] reads when Tag is left empty, and
// the one the package-level [MarshalJSON] and [MarshalWriteJSON] always read.
const defaultTag = "mapstructure"

// Marshaler marshals a Go value whose struct fields are tagged with Tag,
// treating that tag exactly like the package doc comment's semantics
// describe for "mapstructure".
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
	// Tag is the struct tag name to read. Empty means "mapstructure".
	Tag string
}

// tag returns m.Tag, or defaultTag if it is empty.
func (m Marshaler) tag() string {
	if m.Tag == "" {
		return defaultTag
	}

	return m.Tag
}

// Marshal encodes v as JSON.
func (m Marshaler) Marshal(v any) ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := m.MarshalWrite(buf, v); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// MarshalWrite encodes v as JSON, writing it to w.
func (m Marshaler) MarshalWrite(w io.Writer, v any) error {
	// Buffered rather than written to w directly, so the trailing newline
	// jsontext.Encoder leaves after a top-level value — its habit, not this
	// package's — can be trimmed before anything reaches w: Marshal's result
	// should look like encoding/json.Marshal's, not encoding/json's
	// Encoder.Encode.
	buf := &bytes.Buffer{}
	enc := jsontext.NewEncoder(buf, jsontext.WithIndent("  "))

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

	if err := m.encodeValue(enc, rv); err != nil {
		return fmt.Errorf("mapstructurejson: %w", err)
	}

	if _, err := w.Write(bytes.TrimRight(buf.Bytes(), "\n")); err != nil {
		return fmt.Errorf("mapstructurejson: %w", err)
	}

	return nil
}

// MarshalJSON encodes v as JSON, reading struct fields tagged "mapstructure".
// Use [Marshaler] directly to read a different tag.
func MarshalJSON(v any) ([]byte, error) {
	return Marshaler{}.Marshal(v)
}

// MarshalWriteJSON encodes v as JSON, writing it to w, reading struct fields
// tagged "mapstructure". Use [Marshaler] directly to read a different tag.
func MarshalWriteJSON(w io.Writer, v any) error {
	return Marshaler{}.MarshalWrite(w, v)
}
