package openai

import (
	"encoding/json"
	"testing"

	jsonpatch "github.com/evanphx/json-patch"
)

// MergePatch gives the bytes github.com/evanphx/json-patch v0.5.2 gave, the
// merge the SDK used before, for documents and patches as the marshaller
// writes them.
func TestMergePatchMatchesJSONPatch(t *testing.T) {
	type nested struct {
		Z int            `json:"z"`
		A map[string]any `json:"a,omitempty"`
		N *int           `json:"n"`
	}
	docs := []any{
		map[string]any{"model": "m", "stream": true, "messages": []any{map[string]any{"role": "user", "content": "<b>&</b>\u2028"}}},
		struct {
			Model     string `json:"model"`
			Reasoning any    `json:"reasoning,omitempty"`
			Opts      nested `json:"opts"`
			List      []any  `json:"list"`
			Null      any    `json:"null"`
		}{Model: "m", Opts: nested{Z: 1, A: map[string]any{"q": nil, "b": 2}}, List: []any{1, nil, map[string]any{"x": nil}}},
		map[string]any{"z": 1, "a": "è", "m": map[string]any{"k": []any{nil, 1}}, "x": nil},
	}
	patches := []any{
		map[string]any{"enable_thinking": false},
		map[string]any{"reasoning": map[string]any{"effort": "low"}},
		map[string]any{"opts": map[string]any{"z": 2, "a": map[string]any{"q": "v", "b": nil}, "new": map[string]any{"x": nil, "y": []any{nil, 3}}}},
		map[string]any{"model": nil, "list": []any{nil, map[string]any{"k": nil}}, "<&>": "\u2029"},
		map[string]any{"m": map[string]any{"k": map[string]any{"deep": nil}}, "x": map[string]any{"a": nil, "b": 1}, "a": map[string]any{"c": 1}},
		map[string]any{"null": nested{Z: 3, A: map[string]any{"y": 1, "b": nil}}},
		map[string]any{"opts": "replaced", "reasoning": nil},
		map[string]any{"raw": json.RawMessage(`{"b":1,"a":[null,{"c":null}]}`)},
	}
	for _, d := range docs {
		doc, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range patches {
			patch, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			want, wantErr := jsonpatch.MergePatch(doc, patch)
			got, gotErr := MergePatch(doc, patch)
			if (wantErr == nil) != (gotErr == nil) || string(got) != string(want) {
				t.Fatalf("doc %s patch %s:\n got %s (%v)\nwant %s (%v)", doc, patch, got, gotErr, want, wantErr)
			}
		}
	}
}
