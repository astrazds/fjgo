package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const defaultTruncateChars = 1000

type toonTable struct {
	Label  string
	Fields []string
	Rows   []map[string]any
}

type toonBlocks []any

type truncateReport struct {
	Paths []string
}

func (r *truncateReport) Add(path string) {
	r.Paths = append(r.Paths, path)
}

func (r truncateReport) Truncated() bool {
	return len(r.Paths) != 0
}

func writeTOON(w io.Writer, v any) error {
	out, err := marshalTOON(v)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, out)
	return err
}

func marshalTOON(v any) (string, error) {
	normalized, err := normalizeTOONValue(v)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := writeTOONValue(&b, normalized, 0, "result"); err != nil {
		return "", err
	}
	b.WriteByte('\n')
	return b.String(), nil
}

func normalizeTOONValue(v any) (any, error) {
	switch x := v.(type) {
	case toonTable, toonBlocks:
		return canonicalTOONValue(x), nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func canonicalTOONValue(v any) any {
	switch x := v.(type) {
	case toonBlocks:
		out := make(toonBlocks, len(x))
		for i, block := range x {
			out[i] = canonicalTOONValue(block)
		}
		return out
	case toonTable:
		rows := make([]map[string]any, len(x.Rows))
		for i, row := range x.Rows {
			next := make(map[string]any, len(row))
			for key, value := range row {
				next[key] = canonicalTOONValue(value)
			}
			rows[i] = next
		}
		x.Rows = rows
		return x
	case map[string]any:
		out := make(map[string]any, len(x))
		for key, value := range x {
			out[key] = canonicalTOONValue(value)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, value := range x {
			out[i] = canonicalTOONValue(value)
		}
		return out
	}
	rv := reflect.ValueOf(v)
	if rv.IsValid() && rv.Kind() == reflect.Slice {
		out := make([]any, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out = append(out, canonicalTOONValue(rv.Index(i).Interface()))
		}
		return out
	}
	return v
}

func writeJSONAsTOON(w io.Writer, raw []byte, fallbackLabel string, full bool) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return writeTOON(w, map[string]any{"result": "ok"})
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		text := strings.TrimSpace(string(raw))
		if !full {
			var report truncateReport
			text = truncateString(text, defaultTruncateChars, fallbackLabel, &report)
		}
		return writeTOON(w, map[string]any{fallbackLabel: text})
	}
	var report truncateReport
	if !full {
		value = truncateLargeStrings(value, defaultTruncateChars, "", &report)
	}
	blocks := toonBlocks{labeledRoot(fallbackLabel, value)}
	if report.Truncated() {
		blocks = append(blocks, helpBlock([]string{
			fmt.Sprintf("Run `fjgo <same command> --full` to show complete truncated fields (%s)", strings.Join(report.Paths, ", ")),
		}))
	}
	return writeTOON(w, blocks)
}

func labeledRoot(label string, value any) any {
	switch value.(type) {
	case []any:
		return map[string]any{label: value}
	default:
		return value
	}
}

func writeTOONValue(b *strings.Builder, v any, depth int, fallbackLabel string) error {
	switch x := v.(type) {
	case toonBlocks:
		for i, block := range x {
			if i > 0 {
				b.WriteByte('\n')
			}
			if err := writeTOONValue(b, block, depth, fallbackLabel); err != nil {
				return err
			}
		}
	case toonTable:
		writeTOONTable(b, x, depth)
	case map[string]any:
		if len(x) == 0 {
			writeIndent(b, depth)
			b.WriteString("{}")
			return nil
		}
		keys := sortedKeys(x)
		for i, key := range keys {
			if i > 0 {
				b.WriteByte('\n')
			}
			writeKeyedValue(b, key, x[key], depth)
		}
	case []any:
		writeKeyedValue(b, fallbackLabel, x, depth)
	default:
		writeIndent(b, depth)
		b.WriteString(formatScalar(x, ','))
	}
	return nil
}

func writeKeyedValue(b *strings.Builder, key string, value any, depth int) {
	switch x := value.(type) {
	case map[string]any:
		writeIndent(b, depth)
		b.WriteString(key)
		if len(x) == 0 {
			b.WriteString(": {}")
			return
		}
		b.WriteString(":\n")
		_ = writeTOONValue(b, x, depth+1, "items")
	case []any:
		writeArray(b, key, x, depth)
	case nil:
		writeIndent(b, depth)
		b.WriteString(key)
		b.WriteString(": null")
	default:
		writeIndent(b, depth)
		b.WriteString(key)
		b.WriteString(": ")
		b.WriteString(formatScalar(x, ','))
	}
}

func writeArray(b *strings.Builder, key string, values []any, depth int) {
	if len(values) == 0 {
		writeIndent(b, depth)
		fmt.Fprintf(b, "%s[0]:", key)
		return
	}
	if fields, ok := tabularFields(values); ok {
		writeTOONTable(b, toonTable{Label: key, Fields: fields, Rows: rowsFromValues(values)}, depth)
		return
	}
	if primitiveArray(values) {
		writeIndent(b, depth)
		fmt.Fprintf(b, "%s[%d]: ", key, len(values))
		for i, value := range values {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(formatScalar(value, ','))
		}
		return
	}
	writeIndent(b, depth)
	fmt.Fprintf(b, "%s[%d]:", key, len(values))
	for _, value := range values {
		b.WriteByte('\n')
		writeIndent(b, depth+1)
		b.WriteString("- ")
		switch x := value.(type) {
		case map[string]any:
			if len(x) == 0 {
				b.WriteString("{}")
				continue
			}
			keys := sortedKeys(x)
			first := keys[0]
			b.WriteString(first)
			b.WriteString(": ")
			b.WriteString(formatScalar(x[first], ','))
			for _, key := range keys[1:] {
				b.WriteByte('\n')
				writeKeyedValue(b, key, x[key], depth+2)
			}
		default:
			b.WriteString(formatScalar(x, ','))
		}
	}
}

func writeTOONTable(b *strings.Builder, table toonTable, depth int) {
	writeIndent(b, depth)
	fmt.Fprintf(b, "%s[%d]{%s}:", table.Label, len(table.Rows), strings.Join(table.Fields, ","))
	for _, row := range table.Rows {
		b.WriteByte('\n')
		writeIndent(b, depth+1)
		for i, field := range table.Fields {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(formatScalar(row[field], ','))
		}
	}
}

func tabularFields(values []any) ([]string, bool) {
	var fields []string
	for i, value := range values {
		row, ok := value.(map[string]any)
		if !ok || len(row) == 0 {
			return nil, false
		}
		keys := sortedKeys(row)
		for _, key := range keys {
			if !isPrimitive(row[key]) {
				return nil, false
			}
		}
		if i == 0 {
			fields = keys
			continue
		}
		if !reflect.DeepEqual(fields, keys) {
			return nil, false
		}
	}
	return fields, true
}

func rowsFromValues(values []any) []map[string]any {
	rows := make([]map[string]any, 0, len(values))
	for _, value := range values {
		rows = append(rows, value.(map[string]any))
	}
	return rows
}

func primitiveArray(values []any) bool {
	for _, value := range values {
		if !isPrimitive(value) {
			return false
		}
	}
	return true
}

func isPrimitive(value any) bool {
	switch value.(type) {
	case nil, string, bool, json.Number, float64, float32, int, int64, int32, uint, uint64, uint32:
		return true
	default:
		return false
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func writeIndent(b *strings.Builder, depth int) {
	for i := 0; i < depth; i++ {
		b.WriteString("  ")
	}
}

func formatScalar(value any, delimiter rune) string {
	switch x := value.(type) {
	case nil:
		return "null"
	case json.Number:
		return x.String()
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case uint:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case string:
		if shouldQuoteString(x, delimiter) {
			return strconv.Quote(x)
		}
		return x
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return strconv.Quote(fmt.Sprint(x))
		}
		return strconv.Quote(string(b))
	}
}

func shouldQuoteString(s string, delimiter rune) bool {
	if s == "" {
		return true
	}
	if s != strings.TrimSpace(s) {
		return true
	}
	if strings.ContainsRune(s, delimiter) || strings.ContainsAny(s, "\n\r\t") {
		return true
	}
	if strings.ContainsAny(s, ":[]{}#\"") {
		return true
	}
	switch strings.ToLower(s) {
	case "true", "false", "null", "nan", "inf", "-inf":
		return true
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return true
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func truncateLargeStrings(value any, limit int, path string, report *truncateReport) any {
	switch x := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for key, v := range x {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			out[key] = truncateLargeStrings(v, limit, childPath, report)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, v := range x {
			childPath := fmt.Sprintf("[%d]", i)
			if path != "" {
				childPath = fmt.Sprintf("%s[%d]", path, i)
			}
			out[i] = truncateLargeStrings(v, limit, childPath, report)
		}
		return out
	case string:
		return truncateString(x, limit, path, report)
	default:
		return value
	}
}

func truncateString(s string, limit int, path string, report *truncateReport) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	if path == "" {
		path = "result"
	}
	report.Add(path)
	return string(runes[:limit]) + fmt.Sprintf("... (truncated, %d chars total)", len(runes))
}

func tableBlock(label string, fields []string, rows []map[string]any) toonTable {
	return toonTable{Label: label, Fields: fields, Rows: rows}
}

func helpBlock(lines []string) any {
	rows := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		rows = append(rows, map[string]any{"command": line})
	}
	return toonTable{Label: "help", Fields: []string{"command"}, Rows: rows}
}
