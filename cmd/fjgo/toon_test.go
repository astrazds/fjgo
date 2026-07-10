package main

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestMarshalTOONConformingNestedShapes(t *testing.T) {
	got, err := marshalTOON(map[string]any{
		"empty_array":  []any{},
		"empty_object": map[string]any{},
		"matrix":       []any{[]any{1, 2}, []any{}},
		"mixed": []any{
			1,
			map[string]any{"id": 2, "meta": map[string]any{"ok": true}},
			"text",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "empty_array: []\n" +
		"empty_object:\n" +
		"matrix[2]:\n" +
		"  - [2]: 1,2\n" +
		"  - [0]:\n" +
		"mixed[3]:\n" +
		"  - 1\n" +
		"  - id: 2\n" +
		"    meta:\n" +
		"      ok: true\n" +
		"  - text"
	if got != want {
		t.Fatalf("marshalTOON() =\n%s\nwant:\n%s", got, want)
	}
	if strings.HasSuffix(got, "\n") {
		t.Fatal("TOON document has a trailing newline")
	}
}

func TestMarshalTOONListItemWithFirstTabularField(t *testing.T) {
	got, err := marshalTOON([]any{map[string]any{
		"people": []any{
			map[string]any{"id": 1, "name": "Ada"},
			map[string]any{"id": 2, "name": "Bob"},
		},
		"status": "active",
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := "result[1]:\n" +
		"  - people[2]{id,name}:\n" +
		"      1,Ada\n" +
		"      2,Bob\n" +
		"    status: active"
	if got != want {
		t.Fatalf("marshalTOON() =\n%s\nwant:\n%s", got, want)
	}
}

func TestMarshalTOONBlocksOmitEmptyObjectsWithoutBlankLines(t *testing.T) {
	got, err := marshalTOON(toonBlocks{
		map[string]any{},
		map[string]any{"result": "ok"},
		map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "result: ok"; got != want {
		t.Fatalf("marshalTOON() = %q, want %q", got, want)
	}
}

func TestTOONScalarAndKeyEncoding(t *testing.T) {
	tests := map[string]string{
		"hyphen":    formatScalar("-value", ','),
		"plus":      formatScalar("+1", ','),
		"backslash": formatScalar(`a\b`, ','),
		"control":   formatScalar("a\bb", ','),
		"key":       formatKey("not-a-key"),
	}
	wants := map[string]string{
		"hyphen":    `"-value"`,
		"plus":      `"+1"`,
		"backslash": `"a\\b"`,
		"control":   `"a\u0008b"`,
		"key":       `"not-a-key"`,
	}
	for name, got := range tests {
		if got != wants[name] {
			t.Errorf("%s = %q, want %q", name, got, wants[name])
		}
	}
}

func TestTOONCanonicalNumbers(t *testing.T) {
	tests := map[string]string{
		"1e6":                 "1000000",
		"1e-6":                "0.000001",
		"1e-7":                "1e-7",
		"1e21":                "1e+21",
		"-0":                  "0",
		"123.4500":            "123.45",
		"9223372036854775807": "9223372036854775807",
	}
	for input, want := range tests {
		if got := formatScalar(json.Number(input), ','); got != want {
			t.Errorf("canonical number %q = %q, want %q", input, got, want)
		}
	}
}

func TestTOONNormalizesNonFiniteNumbers(t *testing.T) {
	got, err := marshalTOON(map[string]any{
		"infinity": math.Inf(1),
		"nan":      math.NaN(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "infinity: null\nnan: null"; got != want {
		t.Fatalf("marshalTOON() = %q, want %q", got, want)
	}
}

func TestTOONPreservesEncounteredJSONKeyOrder(t *testing.T) {
	input := struct {
		Zulu  int `json:"zulu"`
		Alpha int `json:"alpha"`
	}{Zulu: 1, Alpha: 2}
	got, err := marshalTOON(input)
	if err != nil {
		t.Fatal(err)
	}
	if want := "zulu: 1\nalpha: 2"; got != want {
		t.Fatalf("marshalTOON() = %q, want %q", got, want)
	}
}

func TestJSONAsTOONTabularRowsMayVaryKeyOrder(t *testing.T) {
	var out bytes.Buffer
	raw := []byte(`[{"id":1,"name":"Ada"},{"name":"Bob","id":2}]`)
	if err := writeJSONAsTOON(&out, raw, "people", false); err != nil {
		t.Fatal(err)
	}
	if want := "people[2]{id,name}:\n  1,Ada\n  2,Bob"; out.String() != want {
		t.Fatalf("writeJSONAsTOON() = %q, want %q", out.String(), want)
	}
}

func TestTOONTableObjectCellsPreserveJSONObjectShape(t *testing.T) {
	got, err := marshalTOON(tableBlock("fields", []string{"name", "example"}, []map[string]any{{
		"name": "config",
		"example": map[string]any{
			"mode":   "read",
			"nested": map[string]any{"enabled": true},
		},
	}}))
	if err != nil {
		t.Fatal(err)
	}
	want := `fields[1]{name,example}:
  config,"{\"mode\":\"read\",\"nested\":{\"enabled\":true}}"`
	if got != want || strings.Contains(got, `\"Key\"`) {
		t.Fatalf("marshalTOON() = %q, want %q", got, want)
	}
}
