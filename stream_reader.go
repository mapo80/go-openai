package openai

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"

	utils "github.com/meguminnnnnnnnn/go-openai/internal"
)

// dataPayload is the payload of an SSE data line (the line without its
// "data:" prefix and the white space after it: the \s of RE2, [\t\n\f\r ]),
// and whether line is one.
func dataPayload(line []byte) ([]byte, bool) {
	rest, ok := bytes.CutPrefix(line, []byte("data:"))
	if !ok {
		return nil, false
	}
	return bytes.TrimLeft(rest, "\t\n\f\r "), true
}

// chunkDecoder decodes a stream chunk in one pass, without the generic
// unmarshaler's scan before the type's own decoding.
type chunkDecoder interface {
	decodeChunk(data []byte) error
}

type streamable interface {
	ChatCompletionStreamResponse | CompletionResponse
}

type streamReader[T streamable] struct {
	emptyMessagesLimit uint
	isFinished         bool

	reader         *bufio.Reader
	response       *http.Response
	errAccumulator utils.ErrorAccumulator
	unmarshaler    utils.Unmarshaler

	httpHeader
}

func (stream *streamReader[T]) Recv() (response T, err error) {
	rawLine, err := stream.RecvRaw()
	if err != nil {
		return
	}

	if chunk, ok := any(&response).(chunkDecoder); ok {
		err = chunk.decodeChunk(rawLine)
	} else {
		err = stream.unmarshaler.Unmarshal(rawLine, &response)
	}
	if err != nil {
		return
	}
	return response, nil
}

func (stream *streamReader[T]) RecvRaw() ([]byte, error) {
	if stream.isFinished {
		return nil, io.EOF
	}

	return stream.processLines()
}

//nolint:gocognit
func (stream *streamReader[T]) processLines() ([]byte, error) {
	var (
		emptyMessagesCount uint
		hasErrorPrefix     bool
	)

	for {
		rawLine, readErr := stream.reader.ReadBytes('\n')
		if readErr != nil || hasErrorPrefix {
			respErr := stream.unmarshalError()
			if respErr != nil {
				return nil, fmt.Errorf("error, %w", respErr.Error)
			}
			return nil, readErr
		}

		noSpaceLine := bytes.TrimSpace(rawLine)
		payload, isData := dataPayload(noSpaceLine)
		if isData && bytes.HasPrefix(payload, []byte(`{"error":`)) {
			hasErrorPrefix = true
		}
		if !isData || hasErrorPrefix {
			if hasErrorPrefix && isData {
				noSpaceLine = payload
			}
			writeErr := stream.errAccumulator.Write(noSpaceLine)
			if writeErr != nil {
				return nil, writeErr
			}
			emptyMessagesCount++
			if emptyMessagesCount > stream.emptyMessagesLimit {
				return nil, ErrTooManyEmptyStreamMessages
			}

			continue
		}

		if string(payload) == "[DONE]" {
			stream.isFinished = true
			return nil, io.EOF
		}

		return payload, nil
	}
}

func (stream *streamReader[T]) unmarshalError() (errResp *ErrorResponse) {
	errBytes := stream.errAccumulator.Bytes()
	if len(errBytes) == 0 {
		return
	}

	err := stream.unmarshaler.Unmarshal(errBytes, &errResp)
	if err != nil {
		errResp = nil
	}

	return
}

func (stream *streamReader[T]) Close() error {
	return stream.response.Body.Close()
}
