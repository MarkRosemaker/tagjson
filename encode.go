package mapstructurejson

import (
	"encoding/json/jsontext"
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
func (m Marshaler) encodeValue(enc *jsontext.Encoder, v reflect.Value) error {
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return enc.WriteToken(jsontext.Null)
		}

		v = v.Elem()
	}

	if v.Type() == durationType {
		return enc.WriteToken(jsontext.String(time.Duration(v.Int()).String()))
	}

	switch v.Kind() {
	case reflect.Struct:
		return m.encodeStruct(enc, v)
	case reflect.Map:
		return m.encodeMap(enc, v)
	case reflect.Slice, reflect.Array:
		return m.encodeSlice(enc, v)
	case reflect.String:
		return enc.WriteToken(jsontext.String(v.String()))
	case reflect.Bool:
		return enc.WriteToken(jsontext.Bool(v.Bool()))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return enc.WriteToken(jsontext.Int(v.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return enc.WriteToken(jsontext.Uint(v.Uint()))
	case reflect.Float32:
		return enc.WriteToken(jsontext.Float32(float32(v.Float())))
	case reflect.Float64:
		return enc.WriteToken(jsontext.Float(v.Float()))
	default:
		return fmt.Errorf("cannot encode %s (kind %s)", v.Type(), v.Kind())
	}
}

// encodeStruct writes v as a JSON object, one member per field that carries a
// usable tag and is not empty.
func (m Marshaler) encodeStruct(enc *jsontext.Encoder, v reflect.Value) error {
	if err := enc.WriteToken(jsontext.BeginObject); err != nil {
		return err
	}

	if err := m.writeStructFields(enc, v); err != nil {
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
func (m Marshaler) writeStructFields(enc *jsontext.Encoder, v reflect.Value) error {
	t := v.Type()

	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		tag, key, squash := m.fieldTag(field)
		if !tag {
			continue
		}

		fv := v.Field(i)

		if squash {
			if err := m.writeSquashed(enc, fv); err != nil {
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

		if err := m.encodeValue(enc, fv); err != nil {
			return fmt.Errorf("%s: %w", field.Name, err)
		}
	}

	return nil
}

// fieldTag reads field's m.tag() tag, reporting whether the field is part of
// the schema at all (ok), the key to write it under, and whether it is a
// ",squash" embed. A field with no such tag is treated the same as one
// tagged "-": decoding would not have populated it from a decoded map
// either, so it is not this package's to write back out.
func (m Marshaler) fieldTag(field reflect.StructField) (ok bool, key string, squash bool) {
	tag, present := field.Tag.Lookup(m.tag())
	if !present || tag == "-" {
		return false, "", false
	}

	if tag == ",squash" {
		return true, "", true
	}

	key, _, _ = strings.Cut(tag, ",")

	return true, key, false
}

// writeSquashed inlines an embedded struct's fields into the object currently
// being written. A nil embedded pointer contributes nothing, the same as any
// other empty value would.
func (m Marshaler) writeSquashed(enc *jsontext.Encoder, v reflect.Value) error {
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}

		v = v.Elem()
	}

	return m.writeStructFields(enc, v)
}

// encodeMap writes v as a JSON object, keyed by v's own map keys rather than
// any tag: a map here is a linter name, a module path, a value the caller
// chose, not part of the schema itself. Keys are sorted, since a Go map does
// not otherwise keep the order they went in.
func (m Marshaler) encodeMap(enc *jsontext.Encoder, v reflect.Value) error {
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

		if err := m.encodeValue(enc, mv); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}

	return enc.WriteToken(jsontext.EndObject)
}

// encodeSlice writes v as a JSON array.
func (m Marshaler) encodeSlice(enc *jsontext.Encoder, v reflect.Value) error {
	if err := enc.WriteToken(jsontext.BeginArray); err != nil {
		return err
	}

	for i := range v.Len() {
		if err := m.encodeValue(enc, v.Index(i)); err != nil {
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
	case reflect.Ptr, reflect.Interface:
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
