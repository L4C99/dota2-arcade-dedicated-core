package core

import "testing"

func TestCallerErrorCodes(t *testing.T) {
	m, _, _ := setupManager(t, "ready")
	for _, tc := range []struct {
		method string
		params any
		code   string
	}{
		{"create", map[string]any{"template": "relative.json", "idempotencyKey": "invalid-path"}, "INVALID_PATH"},
		{"logs", map[string]any{"instanceId": "unused", "tail": -1}, "INVALID_REQUEST"},
		{"logs", map[string]any{"instanceId": "unused", "tail": 1001}, "INVALID_REQUEST"},
	} {
		r := call(t, m, tc.method, tc.params)
		if r.OK || r.Error.Code != tc.code || r.Error.Stage != "validate" {
			t.Fatalf("%s: %+v", tc.method, r)
		}
	}
}
