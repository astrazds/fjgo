package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
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

type toonField struct {
	Key   string
	Value any
}

type toonObject []toonField

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
	return b.String(), nil
}

func normalizeTOONValue(v any) (any, error) {
	switch x := v.(type) {
	case toonTable, toonBlocks:
		return canonicalTOONValue(x), nil
	}
	v = normalizeNonFinite(v)
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	return decodeOrderedJSON(dec)
}

func normalizeNonFinite(v any) any {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Interface || rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Float32, reflect.Float64:
		value := rv.Float()
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil
		}
		return rv.Interface()
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return v
		}
		out := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			out[iter.Key().String()] = normalizeNonFinite(iter.Value().Interface())
		}
		return out
	case reflect.Slice, reflect.Array:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			return v
		}
		out := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out[i] = normalizeNonFinite(rv.Index(i).Interface())
		}
		return out
	default:
		return v
	}
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
	case toonObject:
		out := make(toonObject, len(x))
		for i, field := range x {
			out[i] = toonField{Key: field.Key, Value: canonicalTOONValue(field.Value)}
		}
		return out
	case map[string]any:
		keys := sortedKeys(x)
		out := make(toonObject, 0, len(keys))
		for _, key := range keys {
			out = append(out, toonField{Key: key, Value: canonicalTOONValue(x[key])})
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

func decodeOrderedJSON(dec *json.Decoder) (any, error) {
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	switch delim {
	case '{':
		object := toonObject{}
		for dec.More() {
			keyToken, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, fmt.Errorf("TOON object key is not a string")
			}
			value, err := decodeOrderedJSON(dec)
			if err != nil {
				return nil, err
			}
			object = append(object, toonField{Key: key, Value: value})
		}
		_, err = dec.Token()
		return object, err
	case '[':
		array := []any{}
		for dec.More() {
			value, err := decodeOrderedJSON(dec)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		_, err = dec.Token()
		return array, err
	default:
		return nil, fmt.Errorf("unsupported JSON delimiter %q", delim)
	}
}

func writeJSONAsTOON(w io.Writer, raw []byte, fallbackLabel string, full bool) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return writeTOON(w, map[string]any{"result": "ok"})
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	value, err := decodeOrderedJSON(dec)
	if err != nil {
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
		wrote := false
		for _, block := range x {
			var part strings.Builder
			if err := writeTOONValue(&part, block, depth, fallbackLabel); err != nil {
				return err
			}
			if part.Len() == 0 {
				continue
			}
			if wrote {
				b.WriteByte('\n')
			}
			b.WriteString(part.String())
			wrote = true
		}
	case toonTable:
		writeTOONTable(b, x, depth)
	case toonObject:
		writeTOONObject(b, x, depth)
	case map[string]any:
		if len(x) == 0 {
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

func writeTOONObject(b *strings.Builder, object toonObject, depth int) {
	for i, field := range object {
		if i != 0 {
			b.WriteByte('\n')
		}
		writeKeyedValue(b, field.Key, field.Value, depth)
	}
}

func writeKeyedValue(b *strings.Builder, key string, value any, depth int) {
	encodedKey := formatKey(key)
	switch x := value.(type) {
	case toonObject:
		writeIndent(b, depth)
		b.WriteString(encodedKey)
		b.WriteByte(':')
		if len(x) != 0 {
			b.WriteByte('\n')
			writeTOONObject(b, x, depth+1)
		}
	case map[string]any:
		writeIndent(b, depth)
		b.WriteString(encodedKey)
		b.WriteByte(':')
		if len(x) != 0 {
			b.WriteByte('\n')
			_ = writeTOONValue(b, x, depth+1, "items")
		}
	case []any:
		writeArray(b, key, x, depth)
	case nil:
		writeIndent(b, depth)
		b.WriteString(encodedKey)
		b.WriteString(": null")
	default:
		writeIndent(b, depth)
		b.WriteString(encodedKey)
		b.WriteString(": ")
		b.WriteString(formatScalar(x, ','))
	}
}

func writeArray(b *strings.Builder, key string, values []any, depth int) {
	if len(values) == 0 {
		writeIndent(b, depth)
		fmt.Fprintf(b, "%s: []", formatKey(key))
		return
	}
	if fields, ok := tabularFields(values); ok {
		writeTOONTable(b, toonTable{Label: key, Fields: fields, Rows: rowsFromValues(values)}, depth)
		return
	}
	if primitiveArray(values) {
		writeIndent(b, depth)
		fmt.Fprintf(b, "%s[%d]: ", formatKey(key), len(values))
		for i, value := range values {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(formatScalar(value, ','))
		}
		return
	}
	writeIndent(b, depth)
	fmt.Fprintf(b, "%s[%d]:", formatKey(key), len(values))
	for _, value := range values {
		b.WriteByte('\n')
		writeListItem(b, value, depth+1)
	}
}

func writeListItem(b *strings.Builder, value any, depth int) {
	writeIndent(b, depth)
	switch x := value.(type) {
	case toonObject:
		writeObjectFieldsListItem(b, x, depth)
	case map[string]any:
		writeObjectListItem(b, x, depth)
	case []any:
		b.WriteString("- ")
		writeArrayListItem(b, x, depth)
	default:
		b.WriteString("- ")
		b.WriteString(formatScalar(x, ','))
	}
}

func writeArrayListItem(b *strings.Builder, values []any, depth int) {
	if len(values) == 0 {
		b.WriteString("[0]:")
		return
	}
	if primitiveArray(values) {
		fmt.Fprintf(b, "[%d]: ", len(values))
		for i, value := range values {
			if i != 0 {
				b.WriteByte(',')
			}
			b.WriteString(formatScalar(value, ','))
		}
		return
	}
	fmt.Fprintf(b, "[%d]:", len(values))
	for _, value := range values {
		b.WriteByte('\n')
		writeListItem(b, value, depth+1)
	}
}

func writeObjectListItem(b *strings.Builder, object map[string]any, depth int) {
	keys := sortedKeys(object)
	fields := make(toonObject, 0, len(keys))
	for _, key := range keys {
		fields = append(fields, toonField{Key: key, Value: object[key]})
	}
	writeObjectFieldsListItem(b, fields, depth)
}

func writeObjectFieldsListItem(b *strings.Builder, fields toonObject, depth int) {
	if len(fields) == 0 {
		b.WriteByte('-')
		return
	}
	b.WriteString("- ")
	writeFirstListField(b, fields[0].Key, fields[0].Value, depth)
	for _, field := range fields[1:] {
		b.WriteByte('\n')
		writeKeyedValue(b, field.Key, field.Value, depth+1)
	}
}

func writeFirstListField(b *strings.Builder, key string, value any, depth int) {
	b.WriteString(formatKey(key))
	switch x := value.(type) {
	case toonObject:
		b.WriteByte(':')
		if len(x) == 0 {
			return
		}
		b.WriteByte('\n')
		writeTOONObject(b, x, depth+2)
	case map[string]any:
		b.WriteByte(':')
		if len(x) == 0 {
			return
		}
		b.WriteByte('\n')
		_ = writeTOONValue(b, x, depth+2, "items")
	case []any:
		if len(x) == 0 {
			b.WriteString(": []")
			return
		}
		if fields, ok := tabularFields(x); ok {
			fmt.Fprintf(b, "[%d]{%s}:", len(x), formatKeys(fields, ','))
			for _, row := range rowsFromValues(x) {
				b.WriteByte('\n')
				writeIndent(b, depth+2)
				for i, field := range fields {
					if i != 0 {
						b.WriteByte(',')
					}
					b.WriteString(formatScalar(row[field], ','))
				}
			}
			return
		}
		if primitiveArray(x) {
			fmt.Fprintf(b, "[%d]: ", len(x))
			for i, item := range x {
				if i != 0 {
					b.WriteByte(',')
				}
				b.WriteString(formatScalar(item, ','))
			}
			return
		}
		fmt.Fprintf(b, "[%d]:", len(x))
		for _, item := range x {
			b.WriteByte('\n')
			writeListItem(b, item, depth+2)
		}
	default:
		b.WriteString(": ")
		b.WriteString(formatScalar(x, ','))
	}
}

func writeTOONTable(b *strings.Builder, table toonTable, depth int) {
	writeIndent(b, depth)
	fmt.Fprintf(b, "%s[%d]{%s}:", formatKey(table.Label), len(table.Rows), formatKeys(table.Fields, ','))
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
		entries, ok := objectFields(value)
		if !ok || len(entries) == 0 {
			return nil, false
		}
		keys := make([]string, 0, len(entries))
		for _, field := range entries {
			keys = append(keys, field.Key)
			if !isPrimitive(field.Value) {
				return nil, false
			}
		}
		if i == 0 {
			fields = keys
			continue
		}
		if !sameStringSet(fields, keys) {
			return nil, false
		}
	}
	return fields, true
}

func rowsFromValues(values []any) []map[string]any {
	rows := make([]map[string]any, 0, len(values))
	for _, value := range values {
		entries, _ := objectFields(value)
		row := make(map[string]any, len(entries))
		for _, field := range entries {
			row[field.Key] = field.Value
		}
		rows = append(rows, row)
	}
	return rows
}

func objectFields(value any) (toonObject, bool) {
	switch x := value.(type) {
	case toonObject:
		return x, true
	case map[string]any:
		keys := sortedKeys(x)
		fields := make(toonObject, 0, len(keys))
		for _, key := range keys {
			fields = append(fields, toonField{Key: key, Value: x[key]})
		}
		return fields, true
	default:
		return nil, false
	}
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]bool, len(a))
	for _, value := range a {
		seen[value] = true
	}
	for _, value := range b {
		if !seen[value] {
			return false
		}
	}
	return true
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
	case nil, string, bool, json.Number, float64, float32,
		int, int64, int32, int16, int8,
		uint, uint64, uint32, uint16, uint8, uintptr:
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
		return canonicalNumber(x.String())
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return "null"
		}
		return canonicalNumber(strconv.FormatFloat(x, 'g', -1, 64))
	case float32:
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			return "null"
		}
		return canonicalNumber(strconv.FormatFloat(float64(x), 'g', -1, 32))
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case int16:
		return strconv.FormatInt(int64(x), 10)
	case int8:
		return strconv.FormatInt(int64(x), 10)
	case uint:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case uint16:
		return strconv.FormatUint(uint64(x), 10)
	case uint8:
		return strconv.FormatUint(uint64(x), 10)
	case uintptr:
		return strconv.FormatUint(uint64(x), 10)
	case string:
		if shouldQuoteString(x, delimiter) {
			return quoteTOONString(x)
		}
		return x
	case toonObject:
		b, err := json.Marshal(toonJSONValue(x))
		if err != nil {
			return quoteTOONString(fmt.Sprint(x))
		}
		return quoteTOONString(string(b))
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return quoteTOONString(fmt.Sprint(x))
		}
		return quoteTOONString(string(b))
	}
}

func toonJSONValue(value any) any {
	switch value := value.(type) {
	case toonObject:
		object := make(map[string]any, len(value))
		for _, field := range value {
			object[field.Key] = toonJSONValue(field.Value)
		}
		return object
	case []any:
		items := make([]any, len(value))
		for i, item := range value {
			items[i] = toonJSONValue(item)
		}
		return items
	default:
		return value
	}
}

func shouldQuoteString(s string, delimiter rune) bool {
	if s == "" {
		return true
	}
	if s != strings.TrimSpace(s) {
		return true
	}
	if strings.ContainsRune(s, delimiter) || strings.ContainsAny(s, "\n\r\t\\") {
		return true
	}
	if strings.ContainsAny(s, ":[]{}\"") {
		return true
	}
	switch s {
	case "true", "false", "null":
		return true
	}
	if numericLike(s) {
		return true
	}
	if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		return true
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func formatKey(key string) string {
	if validUnquotedKey(key) {
		return key
	}
	return quoteTOONString(key)
}

func formatKeys(keys []string, delimiter rune) string {
	encoded := make([]string, len(keys))
	for i, key := range keys {
		encoded[i] = formatKey(key)
	}
	return strings.Join(encoded, string(delimiter))
}

func validUnquotedKey(key string) bool {
	if key == "" || !asciiLetter(key[0]) && key[0] != '_' {
		return false
	}
	for i := 1; i < len(key); i++ {
		if !asciiLetter(key[i]) && (key[i] < '0' || key[i] > '9') && key[i] != '_' && key[i] != '.' {
			return false
		}
	}
	return true
}

func asciiLetter(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

func numericLike(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	if s[0] == '-' {
		i++
	}
	start := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == start {
		return false
	}
	if i < len(s) && s[i] == '.' {
		i++
		start = i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == start {
			return false
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		start = i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == start {
			return false
		}
	}
	return i == len(s)
}

func quoteTOONString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func canonicalNumber(raw string) string {
	if !numericLike(raw) {
		return "null"
	}
	negative := raw[0] == '-'
	if negative {
		raw = raw[1:]
	}
	mantissa := raw
	exponent := 0
	if i := strings.IndexAny(raw, "eE"); i >= 0 {
		mantissa = raw[:i]
		parsed, err := strconv.Atoi(raw[i+1:])
		if err != nil {
			return "null"
		}
		exponent = parsed
	}
	integer := mantissa
	fraction := ""
	if i := strings.IndexByte(mantissa, '.'); i >= 0 {
		integer, fraction = mantissa[:i], mantissa[i+1:]
	}
	digits := integer + fraction
	first := 0
	for first < len(digits) && digits[first] == '0' {
		first++
	}
	if first == len(digits) {
		return "0"
	}
	last := len(digits)
	for last > first && digits[last-1] == '0' {
		last--
	}
	significant := digits[first:last]
	decimalPosition := len(integer) + exponent - first
	power := decimalPosition - 1
	var out string
	if power >= -6 && power < 21 {
		switch {
		case decimalPosition <= 0:
			out = "0." + strings.Repeat("0", -decimalPosition) + significant
		case decimalPosition >= len(significant):
			out = significant + strings.Repeat("0", decimalPosition-len(significant))
		default:
			out = significant[:decimalPosition] + "." + significant[decimalPosition:]
		}
	} else {
		out = significant[:1]
		if len(significant) > 1 {
			out += "." + significant[1:]
		}
		out += "e"
		if power >= 0 {
			out += "+"
		}
		out += strconv.Itoa(power)
	}
	if negative {
		return "-" + out
	}
	return out
}

func truncateLargeStrings(value any, limit int, path string, report *truncateReport) any {
	switch x := value.(type) {
	case toonObject:
		out := make(toonObject, len(x))
		for i, field := range x {
			childPath := field.Key
			if path != "" {
				childPath = path + "." + field.Key
			}
			out[i] = toonField{Key: field.Key, Value: truncateLargeStrings(field.Value, limit, childPath, report)}
		}
		return out
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
