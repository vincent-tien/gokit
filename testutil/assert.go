package testutil

import (
	"encoding/json"
	"testing"
)

// JSONContains checks that a JSON body contains a key with the expected value.
// Useful for HTTP handler tests where asserting entire body is fragile.
func JSONContains(t testing.TB, body []byte, key string, expected any) {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("JSONContains: unmarshal failed: %v", err)
	}
	val, ok := m[key]
	if !ok {
		t.Fatalf("JSONContains: key %q not found in JSON", key)
	}
	exp, _ := json.Marshal(expected)
	got, _ := json.Marshal(val)
	if string(exp) != string(got) {
		t.Fatalf("JSONContains: key %q: expected %s, got %s", key, exp, got)
	}
}

// JSONEqual checks that two JSON byte slices are semantically equal
// (ignoring key order and whitespace).
func JSONEqual(t testing.TB, expected, actual []byte) {
	t.Helper()
	var e, a any
	if err := json.Unmarshal(expected, &e); err != nil {
		t.Fatalf("JSONEqual: unmarshal expected: %v", err)
	}
	if err := json.Unmarshal(actual, &a); err != nil {
		t.Fatalf("JSONEqual: unmarshal actual: %v", err)
	}
	eb, _ := json.Marshal(e)
	ab, _ := json.Marshal(a)
	if string(eb) != string(ab) {
		t.Fatalf("JSONEqual:\nexpected: %s\nactual:   %s", eb, ab)
	}
}
