package snapshot_test

// A small validator for the JSON Schema keywords api/schemas/sale-snapshot.v1.json uses (type,
// enum, required, properties, additionalProperties: false, items, minItems, minimum, maximum,
// minLength, pattern, oneOf, $ref into $defs), so the Go side tests against the shared contract
// without a new dependency. apps/web/tests/unit/snapshot-schema.ts is the same for TypeScript.

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type schema = map[string]any

func loadSchema(t *testing.T) schema {
	t.Helper()
	dir, _ := os.Getwd()
	for {
		p := filepath.Join(dir, "api", "schemas", "sale-snapshot.v1.json")
		if b, err := os.ReadFile(p); err == nil {
			var s schema
			if err := json.Unmarshal(b, &s); err != nil {
				t.Fatal(err)
			}
			return s
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("api/schemas/sale-snapshot.v1.json not found")
		}
		dir = parent
	}
}

func validate(root schema, doc []byte) error {
	var v any
	if err := json.Unmarshal(doc, &v); err != nil {
		return err
	}
	return check(root, root, v, "$")
}

func typeOf(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case float64:
		if x == math.Trunc(x) {
			return "integer"
		}
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return "unknown"
}

func typeMatches(want any, got string) bool {
	match := func(w string) bool { return w == got || (w == "number" && got == "integer") }
	switch w := want.(type) {
	case string:
		return match(w)
	case []any:
		for _, x := range w {
			if s, _ := x.(string); match(s) {
				return true
			}
		}
	}
	return false
}

func check(root, s schema, v any, path string) error {
	if ref, ok := s["$ref"].(string); ok {
		name := strings.TrimPrefix(ref, "#/$defs/")
		def, _ := root["$defs"].(map[string]any)[name].(map[string]any)
		if def == nil {
			return fmt.Errorf("%s: unknown $ref %s", path, ref)
		}
		return check(root, def, v, path)
	}
	if one, ok := s["oneOf"].([]any); ok {
		n := 0
		var last error
		for _, o := range one {
			if err := check(root, o.(map[string]any), v, path); err == nil {
				n++
			} else {
				last = err
			}
		}
		if n != 1 {
			return fmt.Errorf("%s: matches %d of oneOf (%v)", path, n, last)
		}
		return nil
	}
	got := typeOf(v)
	if t, ok := s["type"]; ok && !typeMatches(t, got) {
		return fmt.Errorf("%s: type %s, want %v", path, got, t)
	}
	if enum, ok := s["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			if fmt.Sprint(e) == fmt.Sprint(v) {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%s: %v not in %v", path, v, enum)
		}
	}
	switch x := v.(type) {
	case float64:
		if m, ok := s["minimum"].(float64); ok && x < m {
			return fmt.Errorf("%s: %v below %v", path, x, m)
		}
		if m, ok := s["maximum"].(float64); ok && x > m {
			return fmt.Errorf("%s: %v above %v", path, x, m)
		}
	case string:
		if m, ok := s["minLength"].(float64); ok && float64(len(x)) < m {
			return fmt.Errorf("%s: shorter than %v", path, m)
		}
		if p, ok := s["pattern"].(string); ok && !regexp.MustCompile(p).MatchString(x) {
			return fmt.Errorf("%s: %q does not match %s", path, x, p)
		}
	case []any:
		if m, ok := s["minItems"].(float64); ok && float64(len(x)) < m {
			return fmt.Errorf("%s: fewer than %v items", path, m)
		}
		if items, ok := s["items"].(map[string]any); ok {
			for i, e := range x {
				if err := check(root, items, e, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}
	case map[string]any:
		props, _ := s["properties"].(map[string]any)
		for _, r := range asList(s["required"]) {
			if _, ok := x[r]; !ok {
				return fmt.Errorf("%s: missing required %q", path, r)
			}
		}
		for k, e := range x {
			ps, ok := props[k].(map[string]any)
			if !ok {
				if ap, ok := s["additionalProperties"].(bool); ok && !ap {
					return fmt.Errorf("%s: unexpected property %q", path, k)
				}
				continue
			}
			if err := check(root, ps, e, path+"."+k); err != nil {
				return err
			}
		}
	}
	return nil
}

func asList(v any) []string {
	l, _ := v.([]any)
	out := make([]string, 0, len(l))
	for _, x := range l {
		out = append(out, x.(string))
	}
	return out
}
