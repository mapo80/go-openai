package openai_test

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/meguminnnnnnnnn/go-openai"
	internal "github.com/meguminnnnnnnnn/go-openai/internal"
	"github.com/meguminnnnnnnnn/go-openai/jsonschema"
)

var requestBodyStrings = []string{
	"", "ciao", "<b>&</b>", "\u2028\u2029", "è €", "\xff\xfe invalid", `"q" \ /` + "\n\t", "\x00\x1f", "{\"not\":\"json\"}",
}

func randomString(r *rand.Rand) string {
	return requestBodyStrings[r.IntN(len(requestBodyStrings))]
}

func randomParameters(r *rand.Rand) any {
	switch r.IntN(5) {
	case 0:
		return nil
	case 1:
		return map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string", "description": randomString(r)}}, "required": []string{"q"}}
	case 2:
		return json.RawMessage(`{"type" : "object",  "x":"\u00e8\/<&>", "n": 1.50E+2, "z":null}`)
	case 3:
		return jsonschema.Definition{Type: jsonschema.Object, Properties: map[string]jsonschema.Definition{"b": {Type: jsonschema.Boolean, Description: randomString(r)}}}
	}
	return []any{1, nil, "s", map[string]any{"k": nil}}
}

func randomMessage(r *rand.Rand) openai.ChatCompletionMessage {
	roles := []string{openai.ChatMessageRoleSystem, openai.ChatMessageRoleUser, openai.ChatMessageRoleAssistant, openai.ChatMessageRoleTool}
	m := openai.ChatCompletionMessage{Role: roles[r.IntN(len(roles))], Name: randomString(r)}
	if r.IntN(4) == 0 {
		m.MultiContent = []openai.ChatMessagePart{
			{Type: openai.ChatMessagePartTypeText, Text: randomString(r)},
			{Type: openai.ChatMessagePartTypeImageURL, ImageURL: &openai.ChatMessageImageURL{URL: "https://x/" + randomString(r), Detail: openai.ImageURLDetailLow}},
		}
	} else {
		m.Content = randomString(r) + strings.Repeat("parola ", r.IntN(40))
	}
	if r.IntN(3) == 0 {
		m.ReasoningContent = randomString(r)
	}
	if m.Role == openai.ChatMessageRoleAssistant && r.IntN(2) == 0 {
		i := r.IntN(3)
		m.ToolCalls = []openai.ToolCall{{Index: &i, ID: "call_" + randomString(r), Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "f", Arguments: `{"a": "` + randomString(r) + `"}`}}}
	}
	if m.Role == openai.ChatMessageRoleTool {
		m.ToolCallID = "call_1"
	}
	return m
}

func randomRequest(r *rand.Rand) openai.ChatCompletionRequest {
	req := openai.ChatCompletionRequest{Model: randomString(r) + "qwen", Stream: r.IntN(2) == 0}
	if r.IntN(8) != 0 {
		for range r.IntN(6) + 1 {
			req.Messages = append(req.Messages, randomMessage(r))
		}
	} else if r.IntN(2) == 0 {
		req.Messages = []openai.ChatCompletionMessage{}
	}
	if r.IntN(2) == 0 {
		for range r.IntN(3) + 1 {
			req.Tools = append(req.Tools, openai.Tool{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{Name: "t" + randomString(r), Description: randomString(r), Strict: r.IntN(2) == 0, Parameters: randomParameters(r)}})
		}
	} else if r.IntN(3) == 0 {
		req.Tools = []openai.Tool{}
	}
	if r.IntN(2) == 0 {
		req.MaxCompletionTokens = r.IntN(4096)
	}
	if r.IntN(3) == 0 {
		req.MaxTokens = r.IntN(100)
	}
	if r.IntN(2) == 0 {
		t := float32(r.IntN(20)) / 10
		req.Temperature = &t
	}
	if r.IntN(3) == 0 {
		req.TopP = 0.5
		req.N = 2
		req.PresencePenalty = -0.25
		req.FrequencyPenalty = 0.75
	}
	if r.IntN(3) == 0 {
		req.Stop = []string{randomString(r), "\n"}
	}
	switch r.IntN(4) {
	case 0:
		req.ResponseFormat = &openai.ChatCompletionResponseFormat{Type: openai.ChatCompletionResponseFormatTypeJSONObject}
	case 1:
		req.ResponseFormat = &openai.ChatCompletionResponseFormat{Type: openai.ChatCompletionResponseFormatTypeJSONSchema, JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
			Name: "out", Description: randomString(r), Schema: json.RawMessage(`{"type":"object", "properties":{"a":{"type":"string"}}}`), Strict: true}}
	}
	if r.IntN(3) == 0 {
		seed := r.IntN(10)
		req.Seed = &seed
		req.LogitBias = map[string]int{"1639": 6, "<&>": -1}
		req.LogProbs = true
		req.TopLogProbs = 2
		req.User = randomString(r)
	}
	switch r.IntN(4) {
	case 0:
		req.ToolChoice = "auto"
	case 1:
		req.ToolChoice = openai.ToolChoice{Type: openai.ToolTypeFunction, Function: openai.ToolFunction{Name: "f"}}
	}
	switch r.IntN(3) {
	case 0:
		req.ParallelToolCalls = false
	case 1:
		req.ParallelToolCalls = true
	}
	if r.IntN(2) == 0 {
		req.StreamOptions = &openai.StreamOptions{IncludeUsage: r.IntN(2) == 0}
	}
	if r.IntN(4) == 0 {
		req.Functions = []openai.FunctionDefinition{{Name: "legacy", Parameters: randomParameters(r)}}
		req.FunctionCall = map[string]any{"name": "legacy"}
	}
	if r.IntN(3) == 0 {
		req.Store = true
		req.ReasoningEffort = "low"
		req.Metadata = map[string]string{"k": randomString(r), "a": "b"}
		req.Prediction = &openai.Prediction{Content: randomString(r), Type: "content"}
		req.ServiceTier = openai.ServiceTierDefault
		req.Verbosity = "low"
		req.SafetyIdentifier = randomString(r)
	}
	if r.IntN(3) == 0 {
		req.ChatTemplateKwargs = map[string]any{"enable_thinking": false, "x": map[string]any{"y": nil}}
	}
	if r.IntN(4) == 0 {
		req.GuidedChoice = []string{"si", randomString(r)}
	}
	return req
}

// randomExtraFields draws from the extra fields ARC sends (enable_thinking,
// reasoning, provider) and from ones that reach the members of the request:
// deleting, replacing and merging them, nested nulls, keys that need escaping
// and two invalid keys that encode to the same name.
func randomExtraFields(r *rand.Rand) map[string]any {
	pool := []func() (string, any){
		func() (string, any) { return "enable_thinking", r.IntN(2) == 0 },
		func() (string, any) {
			return "reasoning", []any{map[string]any{"effort": "low"}, map[string]any{"effort": nil, "max_tokens": 10}}[r.IntN(2)]
		},
		func() (string, any) {
			return "provider", map[string]any{"order": []any{"a", nil, "b"}, "allow_fallbacks": false, "sort": nil}
		},
		func() (string, any) {
			return "messages", []any{nil, []any{map[string]any{"role": "user", "content": "x", "n": nil}}, map[string]any{"a": nil, "b": 1}, "s"}[r.IntN(4)]
		},
		func() (string, any) { return "tools", []any{nil, []any{}, map[string]any{"x": 1, "y": nil}}[r.IntN(3)] },
		func() (string, any) { return "model", []any{nil, "other-model"}[r.IntN(2)] },
		func() (string, any) {
			return "stream_options", []any{nil, map[string]any{"include_usage": nil, "extra": 1}, map[string]any{"include_usage": false}, "x"}[r.IntN(4)]
		},
		func() (string, any) {
			return "response_format", map[string]any{"type": nil, "json_schema": map[string]any{"strict": true, "schema": nil, "new": map[string]any{"z": nil}}}
		},
		func() (string, any) { return "metadata", map[string]any{"k": nil, "new": "v"} },
		func() (string, any) {
			return "chat_template_kwargs", map[string]any{"enable_thinking": nil, "x": map[string]any{"y": 1, "w": nil}}
		},
		func() (string, any) { return "max_completion_tokens", []any{nil, 5}[r.IntN(2)] },
		func() (string, any) { return "<&>\u2028", "\u2029<" },
		func() (string, any) { return "\xff", 1 },
		func() (string, any) { return "\xfe", 2 },
		func() (string, any) { return "messages\xff", true },
		func() (string, any) { return "raw", json.RawMessage(`{"b" : 1, "a":[null,{"c":null}]}`) },
		func() (string, any) { return "guided_choice", []any{nil, []string{"a"}}[r.IntN(2)] },
		func() (string, any) { return "stream", nil },
		func() (string, any) {
			return "tool_choice", map[string]any{"type": nil, "function": map[string]any{"name": "g"}}
		},
	}
	extra := map[string]any{}
	for range r.IntN(5) {
		k, v := pool[r.IntN(len(pool))]()
		extra[k] = v
	}
	return extra
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// The body is the one v0.1.6-arc.1 wrote, byte for byte, for every
// combination of request members and extra fields, and so is the error.
func TestRequestBodyMatchesArc1(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 1))
	m := &internal.JSONMarshaller{}
	withExtras := 0
	for i := range 6000 {
		req := randomRequest(r)
		req.SetExtraFields(randomExtraFields(r))
		if len(req.GetExtraFields()) > 0 {
			withExtras++
		}
		for _, value := range []any{req, &req} {
			want, wantErr := arc1Marshal(value)
			got, gotErr := m.Marshal(value)
			if string(got) != string(want) || errorText(gotErr) != errorText(wantErr) {
				t.Fatalf("case %d, extra fields %v:\n got %s (%v)\nwant %s (%v)", i, req.GetExtraFields(), got, gotErr, want, wantErr)
			}
		}
	}
	if withExtras < 4000 {
		t.Fatalf("only %d cases with extra fields", withExtras)
	}
}

// Errors keep the precedence they had: the request before its extra fields,
// whichever member of the request fails.
func TestRequestBodyErrorsMatchArc1(t *testing.T) {
	misused := openai.ChatCompletionMessage{Role: "user", Content: "a", MultiContent: []openai.ChatMessagePart{{Type: openai.ChatMessagePartTypeText, Text: "b"}}}
	nan := float32(math.NaN())
	badTools := []openai.Tool{{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{Name: "f", Parameters: map[string]any{"x": math.Inf(1)}}}}
	requests := map[string]openai.ChatCompletionRequest{
		"message":        {Model: "m", Messages: []openai.ChatCompletionMessage{{Role: "user", Content: "ok"}, misused}},
		"temperature":    {Model: "m", Temperature: &nan, Messages: []openai.ChatCompletionMessage{{Role: "user", Content: "ok"}}},
		"tool":           {Model: "m", Tools: badTools},
		"message+tool":   {Model: "m", Messages: []openai.ChatCompletionMessage{misused}, Tools: badTools},
		"temperature+tl": {Model: "m", Temperature: &nan, Tools: badTools},
		"valid":          {Model: "m", Messages: []openai.ChatCompletionMessage{{Role: "user", Content: "ok"}}},
	}
	extras := []map[string]any{
		{"enable_thinking": false},
		{"bad": make(chan int)},
		{"bad": math.NaN(), "enable_thinking": true},
		{"messages": nil, "bad": make(chan int)},
		{"messages": []any{"x"}},
		{"tools": nil},
	}
	m := &internal.JSONMarshaller{}
	for name, req := range requests {
		for _, extra := range extras {
			req.SetExtraFields(extra)
			want, wantErr := arc1Marshal(req)
			got, gotErr := m.Marshal(req)
			if string(got) != string(want) || errorText(gotErr) != errorText(wantErr) {
				t.Fatalf("%s with %v:\n got %s (%v)\nwant %s (%v)", name, extra, got, gotErr, want, wantErr)
			}
		}
	}
}

// The bodies the client sends, with and without a body modifier, streaming
// or not, are those of v0.1.6-arc.1.
func TestClientSendsTheArc1RequestBody(t *testing.T) {
	var sent []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent, _ = io.ReadAll(r.Body)
		if strings.Contains(string(sent), `"stream":true`) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"x","choices":[]}`)
	}))
	defer server.Close()
	config := openai.DefaultConfig("token")
	config.BaseURL = server.URL + "/v1"
	client := openai.NewClientWithConfig(config)
	identity := openai.WithRequestBodyModifier(func(body []byte) ([]byte, error) { return body, nil })

	r := rand.New(rand.NewPCG(11, 3))
	for i := range 300 {
		req := randomRequest(r)
		req.Model = "qwen-plus"
		req.ReasoningEffort = ""
		req.MaxTokens = 0
		req.Messages = []openai.ChatCompletionMessage{{Role: "user", Content: randomString(r) + strings.Repeat("parola ", 2000)}}
		req.SetExtraFields(randomExtraFields(r))
		for _, opts := range [][]openai.ChatCompletionRequestOption{nil, {identity}} {
			stream := req
			stream.Stream = true
			want, wantErr := arc1Marshal(stream)
			if wantErr != nil {
				t.Fatalf("case %d: %v", i, wantErr)
			}
			s, err := client.CreateChatCompletionStream(context.Background(), req, opts...)
			if err != nil {
				t.Fatalf("case %d stream: %v", i, err)
			}
			s.Close()
			if string(sent) != string(want) {
				t.Fatalf("case %d stream:\n sent %s\n want %s", i, sent, want)
			}
			plain := req
			plain.Stream = false
			if want, wantErr = arc1Marshal(plain); wantErr != nil {
				t.Fatalf("case %d: %v", i, wantErr)
			}
			if _, err = client.CreateChatCompletion(context.Background(), plain, opts...); err != nil {
				t.Fatalf("case %d: %v", i, err)
			}
			if string(sent) != string(want) {
				t.Fatalf("case %d:\n sent %s\n want %s", i, sent, want)
			}
		}
	}
}

// requestWithExtraFields is a streamed request of about 180 KB, the messages
// and a tool nearly all of it, with the extra field ARC always sends.
func requestWithExtraFields() openai.ChatCompletionRequest {
	req := openai.ChatCompletionRequest{Model: "qwen-plus", Stream: true, MaxCompletionTokens: 2048,
		StreamOptions:  &openai.StreamOptions{IncludeUsage: true},
		ResponseFormat: &openai.ChatCompletionResponseFormat{Type: openai.ChatCompletionResponseFormatTypeJSONObject}}
	for i := range 40 {
		req.Messages = append(req.Messages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: strings.Repeat("contenuto del turno ", 200) + string(rune('a'+i%26))})
	}
	req.Tools = []openai.Tool{{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{Name: "f", Parameters: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string","description":"` + strings.Repeat("d", 20000) + `"}}}`)}}}
	req.SetExtraFields(map[string]any{"enable_thinking": false})
	return req
}

func benchmarkRequestBody(req openai.ChatCompletionRequest) func(*testing.B) {
	return func(b *testing.B) {
		m := &internal.JSONMarshaller{}
		b.ReportAllocs()
		for b.Loop() {
			if _, err := m.Marshal(req); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkRequestBodyWithExtraFields(b *testing.B) {
	benchmarkRequestBody(requestWithExtraFields())(b)
}

// With extra fields the body is written once, as without them: the messages
// and the tools go straight into the request instead of into a document
// merged again. Both are measured alike, so the pooled buffers of
// encoding/json/v2 weigh the same on both sides.
func TestRequestBodyWithExtraFieldsIsWrittenOnce(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector drops pooled buffers at random")
	}
	req := requestWithExtraFields()
	body, err := (&internal.JSONMarshaller{}).Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	plain := req
	plain.SetExtraFields(nil)
	with := testing.Benchmark(benchmarkRequestBody(req)).AllocedBytesPerOp()
	without := testing.Benchmark(benchmarkRequestBody(plain)).AllocedBytesPerOp()
	if extra, limit := with-without, int64(len(body))/4; extra > limit {
		t.Fatalf("a body of %d B allocates %d B with its extra fields, %d without: %d more than the limit %d", len(body), with, without, extra, limit)
	}
}
