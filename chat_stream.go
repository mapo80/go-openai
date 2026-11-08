package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"strings"

	openai "github.com/meguminnnnnnnnn/go-openai/internal"
)

type ChatCompletionStreamChoiceDelta struct {
	Content      string        `json:"content,omitempty"`
	Role         string        `json:"role,omitempty"`
	FunctionCall *FunctionCall `json:"function_call,omitempty"`
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
	Refusal      string        `json:"refusal,omitempty"`

	// This property is used for the "reasoning" feature supported by deepseek-reasoner
	// which is not in the official documentation.
	// the doc from deepseek:
	// - https://api-docs.deepseek.com/api/create-chat-completion#responses
	ReasoningContent string `json:"reasoning_content,omitempty"`

	// Audio is the audio response from the model, returned only when the audio output modality is requested.
	Audio *Audio `json:"audio,omitempty"`

	ExtraFields map[string]json.RawMessage `json:"-"`
}

func (c *ChatCompletionStreamChoiceDelta) UnmarshalJSON(bs []byte) error {
	return jsonv2.Unmarshal(bs, c, openai.DecodeOptions)
}

// UnmarshalJSONFrom decodes a delta in one pass over its tokens: each
// declared member into its field (names matched without case, as before),
// each other member kept as an extra field. A member whose name differs
// only in case from a declared one is both, as when the struct and the extra
// fields were decoded separately.
func (c *ChatCompletionStreamChoiceDelta) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	tok, err := dec.ReadToken()
	if err != nil {
		return err
	}
	var d ChatCompletionStreamChoiceDelta
	switch tok.Kind() {
	case 'n':
		*c = d
		return nil
	case '{':
	default:
		return fmt.Errorf("chat completion delta: a JSON object is expected, not %s", tok.Kind())
	}
	for dec.PeekKind() != '}' {
		tok, err = dec.ReadToken()
		if err != nil {
			return err
		}
		name := tok.String()
		field := d.field(name)
		if field == nil {
			if d.ExtraFields, err = openai.ReadExtraField(dec, nil, name, d.ExtraFields); err != nil {
				return err
			}
			continue
		}
		if deltaNames[name] {
			if err = jsonv2.UnmarshalDecode(dec, field); err != nil {
				return err
			}
			continue
		}
		value, err := dec.ReadValue()
		if err != nil {
			return err
		}
		if d.ExtraFields == nil {
			d.ExtraFields = map[string]json.RawMessage{}
		}
		d.ExtraFields[name] = bytes.Clone(value)
		if err = jsonv2.Unmarshal(value, field, dec.Options()); err != nil {
			return err
		}
	}
	if _, err = dec.ReadToken(); err != nil {
		return err
	}
	*c = d
	return nil
}

var deltaNames = map[string]bool{"content": true, "role": true, "function_call": true, "tool_calls": true,
	"refusal": true, "reasoning_content": true, "audio": true}

// field is the field of the declared member name, matched without case.
func (d *ChatCompletionStreamChoiceDelta) field(name string) any {
	switch {
	case strings.EqualFold(name, "content"):
		return &d.Content
	case strings.EqualFold(name, "role"):
		return &d.Role
	case strings.EqualFold(name, "function_call"):
		return &d.FunctionCall
	case strings.EqualFold(name, "tool_calls"):
		return &d.ToolCalls
	case strings.EqualFold(name, "refusal"):
		return &d.Refusal
	case strings.EqualFold(name, "reasoning_content"):
		return &d.ReasoningContent
	case strings.EqualFold(name, "audio"):
		return &d.Audio
	}
	return nil
}

type ChatCompletionStreamChoiceLogprobs struct {
	Content []ChatCompletionTokenLogprob `json:"content,omitempty"`
	Refusal []ChatCompletionTokenLogprob `json:"refusal,omitempty"`
}

type ChatCompletionTokenLogprob struct {
	Token       string                                 `json:"token"`
	Bytes       []int64                                `json:"bytes,omitempty"`
	Logprob     float64                                `json:"logprob,omitempty"`
	TopLogprobs []ChatCompletionTokenLogprobTopLogprob `json:"top_logprobs"`
}

type ChatCompletionTokenLogprobTopLogprob struct {
	Token   string  `json:"token"`
	Bytes   []int64 `json:"bytes"`
	Logprob float64 `json:"logprob"`
}

type ChatCompletionStreamChoice struct {
	Index                int                                 `json:"index"`
	Delta                ChatCompletionStreamChoiceDelta     `json:"delta"`
	Logprobs             *ChatCompletionStreamChoiceLogprobs `json:"logprobs,omitempty"`
	FinishReason         FinishReason                        `json:"finish_reason"`
	ContentFilterResults ContentFilterResults                `json:"content_filter_results,omitempty"`
}

type PromptFilterResult struct {
	Index                int                  `json:"index"`
	ContentFilterResults ContentFilterResults `json:"content_filter_results,omitempty"`
}

type ChatCompletionStreamResponse struct {
	ID                  string                       `json:"id"`
	Object              string                       `json:"object"`
	Created             int64                        `json:"created"`
	Model               string                       `json:"model"`
	Choices             []ChatCompletionStreamChoice `json:"choices"`
	SystemFingerprint   string                       `json:"system_fingerprint"`
	PromptAnnotations   []PromptAnnotation           `json:"prompt_annotations,omitempty"`
	PromptFilterResults []PromptFilterResult         `json:"prompt_filter_results,omitempty"`
	// An optional field that will only be present when you set stream_options: {"include_usage": true} in your request.
	// When present, it contains a null value except for the last chunk which contains the token usage statistics
	// for the entire request.
	Usage   *Usage `json:"usage,omitempty"`
	RawBody []byte `json:"-"`
}

func (c *ChatCompletionStreamResponse) UnmarshalJSON(data []byte) error {
	return c.decodeChunk(data)
}

// decodeChunk decodes one chunk of the stream in a single pass (json/v2,
// the delta by its tokens) and keeps data as RawBody.
func (c *ChatCompletionStreamResponse) decodeChunk(data []byte) error {
	type chatCompletionStreamResponse ChatCompletionStreamResponse
	response := chatCompletionStreamResponse{}

	err := jsonv2.Unmarshal(data, &response, openai.DecodeOptions)
	if err != nil {
		return err
	}

	response.RawBody = data
	*c = ChatCompletionStreamResponse(response)

	return nil
}

// ChatCompletionStream
// Note: Perhaps it is more elegant to abstract Stream using generics.
type ChatCompletionStream struct {
	*streamReader[ChatCompletionStreamResponse]
}

// CreateChatCompletionStream — API call to create a chat completion w/ streaming
// support. It sets whether to stream back partial progress. If set, tokens will be
// sent as data-only server-sent events as they become available, with the
// stream terminated by a data: [DONE] message.
func (c *Client) CreateChatCompletionStream(
	ctx context.Context,
	request ChatCompletionRequest,
	opts ...ChatCompletionRequestOption,
) (stream *ChatCompletionStream, err error) {
	urlSuffix := chatCompletionsSuffix
	if !checkEndpointSupportsModel(urlSuffix, request.Model) {
		err = ErrChatCompletionInvalidModel
		return
	}

	request.Stream = true
	reasoningValidator := NewReasoningValidator()
	if err = reasoningValidator.Validate(request); err != nil {
		return
	}

	ccOpts := &chatCompletionRequestOptions{}
	for _, opt := range opts {
		opt(ccOpts)
	}

	body := any(request)
	if ccOpts.RequestBodyModifier != nil {
		var newBody io.Reader
		newBody, err = c.getNewRequestBody(request, ccOpts.RequestBodyModifier)
		if err != nil {
			return stream, err
		}
		body = newBody
	}

	req, err := c.newRequest(
		ctx,
		http.MethodPost,
		c.fullURL(urlSuffix, withModel(request.Model)),
		withBody(body),
		withExtraHeader(ccOpts.ExtraHeader),
	)
	if err != nil {
		return nil, err
	}

	resp, err := sendRequestStream[ChatCompletionStreamResponse](c, req)
	if err != nil {
		return
	}
	stream = &ChatCompletionStream{
		streamReader: resp,
	}
	return
}
