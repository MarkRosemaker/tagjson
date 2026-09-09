package mapstructurejson_test

import (
	"bytes"
	"encoding/json/jsontext"
	"strings"
	"testing"
	"time"

	"github.com/MarkRosemaker/json2yaml"
	"github.com/MarkRosemaker/portfolio/mapstructurejson"
	"github.com/golangci/golangci-lint/v2/pkg/config"
	"gopkg.in/yaml.v3"
)

// marshalYAML exercises the package's documented recipe for YAML — MarshalJSON,
// then any JSON-to-YAML converter — kept here rather than in the package so
// mapstructurejson's own dependency list stays JSON-only.
func marshalYAML(t *testing.T, v any) []byte {
	t.Helper()

	b, err := mapstructurejson.MarshalJSON(v)
	if err != nil {
		t.Fatal(err)
	}

	node, err := json2yaml.Convert(jsontext.Value(b))
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := yaml.NewEncoder(&out).Encode(node); err != nil {
		t.Fatal(err)
	}

	return out.Bytes()
}

// TestMarshalYAMLFormatterExclusions is the bug that started this package: an
// untranslated golangci-lint config type marshaled with capitalised Go field
// names and every zero value spelled out, because it lacked a hand-registered
// shadow type. The reflection-driven encoder has no such registry to miss an
// entry from, but the case is worth keeping as a named regression test rather
// than trusting that property to hold implicitly.
func TestMarshalYAMLFormatterExclusions(t *testing.T) {
	for _, tc := range []struct {
		name       string
		exclusions config.FormatterExclusions
		want       []string
		dontWant   []string
	}{
		{
			name:       "only Generated set",
			exclusions: config.FormatterExclusions{Generated: "strict"},
			want:       []string{"generated: strict"},
			dontWant:   []string{"Generated", "paths:", "Paths", "warn-unused", "WarnUnused"},
		},
		{
			name: "Paths and WarnUnused set too",
			exclusions: config.FormatterExclusions{
				Generated:  "strict",
				Paths:      []string{"vendor/.*"},
				WarnUnused: true,
			},
			want:     []string{"generated: strict", "paths:", "vendor/.*", "warn-unused: true"},
			dontWant: []string{"Generated", "Paths", "WarnUnused"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Config{Formatters: config.Formatters{Exclusions: tc.exclusions}}

			got := marshalYAML(t, cfg)

			for _, want := range tc.want {
				if !strings.Contains(string(got), want) {
					t.Errorf("output is missing %q:\n%s", want, got)
				}
			}

			for _, bad := range tc.dontWant {
				if strings.Contains(string(got), bad) {
					t.Errorf("output leaked an untranslated Go field name %q:\n%s", bad, got)
				}
			}
		})
	}
}

// TestMarshalRealisticConfig exercises the encoder against the shape of
// config this repo actually generates: a handful of linters and formatters
// enabled with real settings, nothing else. It is a much smaller check of
// "does the whole pipeline still work end to end" than of any one mechanic —
// those are covered in encode_test.go — and pins down the exact YAML this
// repo's generator (internal/lintgen) depends on not shifting silently.
func TestMarshalRealisticConfig(t *testing.T) {
	cfg := config.Config{
		Version: "2",
		Run:     config.Run{Timeout: 5 * time.Minute},
		Linters: config.Linters{
			Default: config.GroupNone,
			Enable:  []string{"usetesting", "tagalign"},
			Settings: config.LintersSettings{
				UseTesting: config.UseTestingSettings{OSChdir: true},
				TagAlign: config.TagAlignSettings{
					Align: true,
					Order: []string{"json", "required"},
				},
			},
		},
		Formatters: config.Formatters{
			Enable:     []string{"gofumpt"},
			Exclusions: config.FormatterExclusions{Generated: "strict"},
		},
	}

	got := marshalYAML(t, cfg)

	want := `version: 2
run:
    timeout: 5m0s
linters:
    default: none
    enable:
        - usetesting
        - tagalign
    settings:
        tagalign:
            align: true
            order:
                - json
                - required
        usetesting:
            os-chdir: true
formatters:
    enable:
        - gofumpt
    exclusions:
        generated: strict
`

	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestMarshalWriteJSONHasNoTrailingNewline matches encoding/json.Marshal's
// convention (no trailing newline), rather than encoding/json.Encoder.Encode's
// (one) — the underlying jsontext.Encoder always adds one after a top-level
// value, which MarshalJSON and MarshalWriteJSON trim before returning.
func TestMarshalWriteJSONHasNoTrailingNewline(t *testing.T) {
	got, err := mapstructurejson.MarshalJSON(config.Config{Version: "2"})
	if err != nil {
		t.Fatal(err)
	}

	if strings.HasSuffix(string(got), "\n") {
		t.Errorf("output has a trailing newline: %q", got)
	}
}

// TestMarshalInternalFieldsAreExcluded checks that config.Config's own
// InternalTest and InternalCmdTest — present for golangci-lint's use, not
// part of the on-disk schema, and carrying no mapstructure tag at all — never
// reach the output, however they are set.
func TestMarshalInternalFieldsAreExcluded(t *testing.T) {
	cfg := config.Config{Version: "2"}
	cfg.InternalTest = true
	cfg.InternalCmdTest = true

	got, err := mapstructurejson.MarshalJSON(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(got), "nternal") {
		t.Errorf("an internal field leaked into the output: %s", got)
	}
}

// TestMarshalYAML reproduces a real crash: under `go test -race`, this exact
// call chain (marshalYAML -> MarshalJSON -> MarshalWriteJSON, then a
// JSON-to-YAML conversion on the result) reliably corrupted the heap ("found
// bad pointer in Go heap") a few frames into encodeValue, and did not
// reproduce calling MarshalJSON directly at the same nesting depth without
// also converting to YAML afterwards. Reproduces on go1.27 both with and
// without GOEXPERIMENT=jsonv2, so it is not the experiment; it reads as a
// toolchain escape-analysis defect for boxing a value this large into
// reflect.ValueOf's any parameter at depth, not a bug in the value being
// encoded. See the comment on reflect.ValueOf(&v).Elem() in MarshalWriteJSON
// for the fix. This test only means anything under `go test -race`; run it
// that way, repeated, when touching this package's entry points.
func TestMarshalYAML(t *testing.T) {
	cfg := config.Config{
		Linters: config.Linters{
			Settings: config.LintersSettings{
				TagAlign: config.TagAlignSettings{Order: []string{"json", "required"}},
			},
		},
	}

	for range 5 {
		got := marshalYAML(t, cfg)

		if !strings.Contains(string(got), "- json") {
			t.Errorf("got:\n%s", got)
		}
	}
}
