package openai

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// referenceDelta decodes a delta as the SDK did before json/v2: the declared
// fields with encoding/json, then every member whose name is not exactly a
// declared one as an extra field (none: nil).
func referenceDelta(t *testing.T, data string) (ChatCompletionStreamChoiceDelta, error) {
	t.Helper()
	type plain struct {
		Content          string        `json:"content,omitempty"`
		Role             string        `json:"role,omitempty"`
		FunctionCall     *FunctionCall `json:"function_call,omitempty"`
		ToolCalls        []ToolCall    `json:"tool_calls,omitempty"`
		Refusal          string        `json:"refusal,omitempty"`
		ReasoningContent string        `json:"reasoning_content,omitempty"`
		Audio            *Audio        `json:"audio,omitempty"`
	}
	var p plain
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		return ChatCompletionStreamChoiceDelta{}, err
	}
	d := ChatCompletionStreamChoiceDelta{Content: p.Content, Role: p.Role, FunctionCall: p.FunctionCall, ToolCalls: p.ToolCalls,
		Refusal: p.Refusal, ReasoningContent: p.ReasoningContent, Audio: p.Audio}
	var all map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &all); err != nil {
		return ChatCompletionStreamChoiceDelta{}, err
	}
	for name, value := range all {
		if !deltaNames[name] {
			if d.ExtraFields == nil {
				d.ExtraFields = map[string]json.RawMessage{}
			}
			d.ExtraFields[name] = value
		}
	}
	return d, nil
}

func TestStreamDeltaDecodesAsBeforeInOnePass(t *testing.T) {
	for _, data := range []string{
		`{}`,
		`null`,
		`{"content":"ciao","role":"assistant"}`,
		`{"content":"","reasoning_content":"penso"}`,
		`{"reasoning":"penso altrove","content":""}`,
		`{"reasoning_details":[{"type":"reasoning.text","text":"a"}],"content":"x"}`,
		`{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"f","arguments":"{\"a\":1}"}}]}`,
		`{"tool_calls":[{"index":0,"function":{"arguments":"1}"}}]}`,
		`{"function_call":{"name":"f","arguments":"{}"},"refusal":"no"}`,
		`{"audio":{"id":"a","data":"ZGF0YQ==","expires_at":12,"transcript":"t"}}`,
		`{"Content":"maiuscolo","content":"minuscolo"}`,
		`{"content":"minuscolo","CONTENT":"maiuscolo"}`,
		`{"content":"uno","content":"due"}`,
		`{"content":null,"role":null,"tool_calls":null}`,
		`{"content":"\u00e8 \ud83d\ude00 \"citato\""}`,
		`{"extra":{"nested":[1,2.50,"x",true,null]},"n":1e3}`,
		"{\"content\":\"\xff\xfe\"}",
		`{"content":1}`,
		`{"tool_calls":{}}`,
		`[]`,
	} {
		want, wantErr := referenceDelta(t, data)
		var got ChatCompletionStreamChoiceDelta
		gotErr := got.UnmarshalJSON([]byte(data))
		if (wantErr == nil) != (gotErr == nil) {
			t.Fatalf("%s: error %v, before %v", data, gotErr, wantErr)
		}
		if wantErr != nil {
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s:\n got %+v\nwant %+v", data, got, want)
		}
	}
}

// A chunk decoded by the stream keeps its line as RawBody and its delta's
// extra fields as their own copies.
func TestStreamChunkKeepsRawBodyAndOwnExtras(t *testing.T) {
	line := []byte(`{"id":"c","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"content":"a","reasoning":"r"},"finish_reason":null}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5,"prompt_tokens_details":{"cached_tokens":1},"cost":0.5}}`)
	var c ChatCompletionStreamResponse
	if err := c.decodeChunk(line); err != nil {
		t.Fatal(err)
	}
	if &c.RawBody[0] != &line[0] || len(c.RawBody) != len(line) {
		t.Fatal("RawBody is not the chunk's line")
	}
	extra := c.Choices[0].Delta.ExtraFields["reasoning"]
	copy(line, strings.Repeat("x", len(line)))
	if string(extra) != `"r"` || c.Choices[0].Delta.Content != "a" {
		t.Fatalf("delta %+v", c.Choices[0].Delta)
	}
	if c.Usage == nil || c.Usage.TotalTokens != 5 || c.Usage.PromptTokensDetails.CachedTokens != 1 || string(c.Usage.ExtraFields["cost"]) != `0.5` {
		t.Fatalf("usage %+v", c.Usage)
	}
}

func TestDataPayloadMatchesTheSSEPrefix(t *testing.T) {
	for line, want := range map[string]string{
		"data: {}":      "{}",
		"data:{}":       "{}",
		"data: \t\f {}": "{}",
		"data: [DONE]":  "[DONE]",
		"data:":         "",
		"event: x":      "<none>",
		": comment":     "<none>",
		"Data: {}":      "<none>",
		"data:\v{}":     "\v{}",
	} {
		got, ok := dataPayload([]byte(line))
		if !ok {
			got = []byte("<none>")
		}
		if string(got) != want {
			t.Errorf("%q: %q, want %q", line, got, want)
		}
	}
}
