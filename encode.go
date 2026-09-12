package tagjson

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

// durationType lets encodeValue recognise a time.Duration by its exact type,
// rather than by its underlying int64 kind, which every other integer type
// shares.
var durationType = reflect.TypeFor[time.Duration]()

// encodeValue writes v to enc.
//
// It is the callee's job to decide *how* to write a value once asked to;
// deciding *whether* to — whether v is empty and so left out entirely — is
// the caller's, made once against [Marshaler.isEmpty] before recursing here,
// since only the caller (holding the struct field or map entry v came from)
// can act on that answer by omitting the key altogether.
func (m Marshaler) encodeValue(enc *jsontext.Encoder, v reflect.Value, opts ...json.Options) error {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return enc.WriteToken(jsontext.Null)
		}

		v = v.Elem()
	default:
	}

	switch v.Kind() {
	case reflect.Struct:
		return m.encodeStruct(enc, v, opts...)
	case reflect.Map:
		return m.encodeMap(enc, v, opts...)
	case reflect.Slice, reflect.Array:
		return m.encodeSlice(enc, v, opts...)
	default:
		return json.MarshalEncode(enc, v.Interface(), opts...)
	}
}

// encodeStruct writes v as a JSON object, one member per field that carries a
// usable tag and is not empty.
func (m Marshaler) encodeStruct(enc *jsontext.Encoder, v reflect.Value, opts ...json.Options) error {
	if err := enc.WriteToken(jsontext.BeginObject); err != nil {
		return err
	}

	if err := m.writeStructFields(enc, v, opts...); err != nil {
		return err
	}

	return enc.WriteToken(jsontext.EndObject)
}

// writeStructFields writes v's fields as object members, without the
// enclosing braces. It is called both for a struct in its own right
// (encodeStruct wraps it in BeginObject/EndObject) and for one embedded with
// tag ",squash", whose fields belong to the *enclosing* object instead of one
// of their own — so this is also where squashing happens: a squashed field is
// expanded via a second call to writeStructFields, at the position it was
// declared, rather than written as a nested value.
func (m Marshaler) writeStructFields(enc *jsontext.Encoder, v reflect.Value, opts ...json.Options) error {
	t := v.Type()

	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		tag, key, inline := m.fieldTag(field)
		if !tag {
			continue
		}

		fv := v.Field(i)

		if inline {
			if err := m.writeInlined(enc, fv, opts...); err != nil {
				return fmt.Errorf("%s: %w", field.Name, err)
			}

			continue
		}

		if m.isEmpty(fv) {
			continue
		}

		if err := enc.WriteToken(jsontext.String(key)); err != nil {
			return err
		}

		if err := m.encodeValue(enc, fv, opts...); err != nil {
			return fmt.Errorf("%s: %w", field.Name, err)
		}
	}

	return nil
}

// fieldTag reads field's m.tag tag, reporting whether the field is part of
// the schema at all (ok), the key to write it under, and whether it is an embed.
// A field with no such tag is treated the same as one
// tagged "-": decoding would not have populated it from a decoded map
// either, so it is not this package's to write back out.
func (m Marshaler) fieldTag(field reflect.StructField) (bool, string, bool) {
	tag, present := field.Tag.Lookup(m.tag)
	if !present || tag == "-" {
		return false, "", false
	}

	key, opt, _ := strings.Cut(tag, ",")
	switch opt {
	case "squash", "inline", "embed":
		return true, "", true
	}

	return true, key, false
}

// writeInlined inlines an embedded struct's fields into the object currently
// being written. A nil embedded pointer contributes nothing, the same as any
// other empty value would.
func (m Marshaler) writeInlined(enc *jsontext.Encoder, v reflect.Value, opts ...json.Options) error {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}

		v = v.Elem()
	}

	return m.writeStructFields(enc, v, opts...)
}

// encodeMap writes v as a JSON object, keyed by v's own map keys rather than
// any tag: a map here is a linter name, a module path, a value the caller
// chose, not part of the schema itself. Keys are sorted, since a Go map does
// not otherwise keep the order they went in.
func (m Marshaler) encodeMap(enc *jsontext.Encoder, v reflect.Value, opts ...json.Options) error {
	if err := enc.WriteToken(jsontext.BeginObject); err != nil {
		return err
	}

	keys := v.MapKeys()
	sort.Slice(keys, func(i, j int) bool {
		return fmt.Sprint(keys[i].Interface()) < fmt.Sprint(keys[j].Interface())
	})

	for _, k := range keys {
		mv := v.MapIndex(k)
		if m.isEmpty(mv) {
			continue
		}

		key := fmt.Sprint(k.Interface())

		if err := enc.WriteToken(jsontext.String(key)); err != nil {
			return err
		}

		if err := m.encodeValue(enc, mv, opts...); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}

	return enc.WriteToken(jsontext.EndObject)
}

// encodeSlice writes v as a JSON array.
func (m Marshaler) encodeSlice(enc *jsontext.Encoder, v reflect.Value, opts ...json.Options) error {
	if err := enc.WriteToken(jsontext.BeginArray); err != nil {
		return err
	}

	for i := range v.Len() {
		if err := m.encodeValue(enc, v.Index(i), opts...); err != nil {
			return fmt.Errorf("[%d]: %w", i, err)
		}
	}

	return enc.WriteToken(jsontext.EndArray)
}

// isEmpty reports whether v is empty enough to leave out of the output
// altogether: the zero value for a scalar, nil for a pointer or interface, no
// elements for a map or slice, or — for a struct — every field it would
// otherwise write likewise empty. It uses the same field-selection rule as
// writeStructFields (skipping fields with no usable tag) so the two never
// disagree about what a struct actually contributes: a struct is empty
// exactly when nothing about it would be written.
func (m Marshaler) isEmpty(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		return v.IsNil() || m.isEmpty(v.Elem())
	case reflect.Map, reflect.Slice:
		return v.Len() == 0
	case reflect.Struct:
		if v.Type() == durationType {
			return v.Int() == 0
		}

		t := v.Type()

		for i := range t.NumField() {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}

			if ok, _, _ := m.fieldTag(field); !ok {
				continue
			}

			if !m.isEmpty(v.Field(i)) {
				return false
			}
		}

		return true
	default:
		return v.IsZero()
	}
}
