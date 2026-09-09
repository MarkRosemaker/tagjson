# tagjson

Marshal a struct-tagged Go value to JSON (e.g. [mapstructure](https://github.com/go-viper/mapstructure)-tagged) — reading the tags themselves, not a parallel set of `json` tags.

```go
type Settings struct {
	TabLen int  `mapstructure:"tab-len"`
	Fast   bool `mapstructure:"fast"`
}

b, _ := tagjson.MarshalJSON(Settings{TabLen: 4})
// {"tab-len":4}
```

## Why this exists

`mapstructure` is decode-only: it reads a plain map (typically itself decoded
from YAML or JSON) into a Go struct tagged for it, but has no marshal
direction of its own. Handing such a struct to `encoding/json` directly
ignores the `mapstructure` tags entirely, falling back to raw Go field names
and writing every zero value out explicitly.

`MarshalJSON` reads the `mapstructure` tag through reflection instead, so any
type already set up for `mapstructure` decoding marshals back out correctly
with no further annotation — including a type added after this package was
last touched, since there is no per-type registry to fall out of sync.

## Reading a different tag

`MarshalJSON` and `MarshalWriteJSON` always read `mapstructure`, matching the
package name. For any other tag — `yaml`, `toml`, a project's own — use
`Marshaler` directly, with the same semantics throughout:

```go
type Settings struct {
	TabLen int  `yaml:"tab-len"`
	Fast   bool `yaml:"fast"`
}

m := tagjson.Marshaler{Tag: "yaml"}
b, _ := m.Marshal(Settings{TabLen: 4})
// {"tab-len":4}
```

`Marshaler{}` (Tag left empty) behaves exactly like the package-level
functions. Its methods are named `Marshal`/`MarshalWrite`, not
`MarshalJSON`/`MarshalWriteJSON` — a method actually named `MarshalJSON` is
what `encoding/json` looks for to mean "this type knows how to marshal
itself", the opposite of what `Marshaler` does.

## Getting YAML

This package only produces JSON: JSON is what every YAML library already
round-trips through, so a caller wanting YAML converts `MarshalJSON`'s output
with whichever library they prefer, rather than this package choosing one and
every consumer paying for it:

```go
import (
	"encoding/json/jsontext"

	"github.com/MarkRosemaker/json2yaml"
	"github.com/MarkRosemaker/portfolio/tagjson"
	"gopkg.in/yaml.v3"
)

b, err := tagjson.MarshalJSON(v)
node, err := json2yaml.Convert(jsontext.Value(b))
yaml.NewEncoder(os.Stdout).Encode(node)
```

## Semantics

The mapping follows `mapstructure`'s own rules, in reverse:

- A field tagged `"-"`, or carrying no `mapstructure` tag at all, is
  something `mapstructure` would not have populated from a decoded map
  either, and is omitted.
- A field tagged `",squash"` is an embedded struct whose own fields are
  inlined into the enclosing object, rather than nested under a key of their
  own — `mapstructure`'s flattening, run the other direction.
- Every other tag names the field's key verbatim.
- A field left at its zero value — an empty string, an unset bool, a nil or
  empty slice or map, a struct whose own fields are all likewise empty — is
  omitted.
- Map keys are written as-is, sorted, since they're caller-chosen names
  (not fixed fields) and a Go map has no order of its own.
- `time.Duration` is written as its `String()` form (`"1h30m"`), not as a
  raw number of nanoseconds.

## What this package can't do

There is no way, from the `mapstructure` tag alone, to know whether a
particular type's zero value happens to *be* its meaningful default. A type
where that distinction matters — some field defaults to `true`, say, so an
explicit `false` must never be confused with "unset" — needs a hand-written
marshaler for that one type instead of this package's generic, reflection-driven
one. See [`lintconfig`](../lintconfig) for exactly that: the same problem,
solved by hand for golangci-lint's config type, field by field, with the
build breaking instead of drifting silently when golangci-lint's own types
change shape.

## A known toolchain issue

`MarshalWriteJSON` calls `reflect.ValueOf(&v).Elem().Elem()` rather than the
more obvious `reflect.ValueOf(v)`. The plain form reproducibly crashed a
`go test -race` build with "found bad pointer in Go heap" a few call frames
into the reflection walk — reached through the "Getting YAML" call chain above
(`MarshalJSON` followed by a JSON-to-YAML conversion) but not through
`MarshalJSON` called directly at the same depth. It reproduces on go1.27 both
with and without `GOEXPERIMENT=jsonv2`, so it reads as a toolchain
escape-analysis defect for boxing a large value into `reflect.ValueOf`'s `any`
parameter at depth, not a bug in the value being encoded. See the comment
beside the fix and `TestMarshalYAML`, and re-check both once this project's Go
toolchain is past its current bleeding edge.
