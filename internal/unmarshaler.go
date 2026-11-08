package openai

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// DecodeOptions are the options of the decodings the SDK makes directly
// with encoding/json/v2: those of encoding/json, so that one library decodes
// with the semantics the SDK always had.
var DecodeOptions = json.DefaultOptionsV1()

type Unmarshaler interface {
	Unmarshal(data []byte, v any) error
}

type JSONUnmarshaler struct{}

func (jm *JSONUnmarshaler) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

var declaredNames sync.Map // reflect.Type -> map[string]bool

// DeclaredNames are the JSON names of the fields of struct type typ (its
// json tag name, or the Go name of an exported field without tag): the
// members that are not extra fields.
func DeclaredNames(typ reflect.Type) (map[string]bool, error) {
	for typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("type is not a struct")
	}
	if names, ok := declaredNames.Load(typ); ok {
		return names.(map[string]bool), nil
	}
	names := map[string]bool{}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if jsonTag := field.Tag.Get("json"); jsonTag != "" {
			name, _, _ := strings.Cut(jsonTag, ",")
			if name != "-" {
				names[name] = true
			}
		} else if field.IsExported() {
			names[field.Name] = true
		}
	}
	declaredNames.Store(typ, names)
	return names, nil
}

// UnmarshalExtraFields returns the members of the JSON object data whose
// names are not declared by struct type typ, each with its own copy of its
// value; nil when there is none. Members are read as tokens: only the
// values of the extra members are kept.
func UnmarshalExtraFields(typ reflect.Type, data []byte) (map[string]json.RawMessage, error) {
	names, err := DeclaredNames(typ)
	if err != nil {
		return nil, err
	}
	dec := jsontext.NewDecoder(bytes.NewBuffer(data), DecodeOptions)
	tok, err := dec.ReadToken()
	if err != nil {
		return nil, err
	}
	switch tok.Kind() {
	case 'n':
		return nil, nil
	case '{':
	default:
		return nil, fmt.Errorf("extra fields of %s: a JSON object is expected", tok.Kind())
	}
	var extra map[string]json.RawMessage
	for dec.PeekKind() != '}' {
		name, err := dec.ReadToken()
		if err != nil {
			return nil, err
		}
		if extra, err = ReadExtraField(dec, names, name.String(), extra); err != nil {
			return nil, err
		}
	}
	if _, err := dec.ReadToken(); err != nil {
		return nil, err
	}
	return extra, nil
}

// ReadExtraField reads the value of member name: kept, as its own copy, in
// extra when name is not declared, skipped otherwise.
func ReadExtraField(dec *jsontext.Decoder, names map[string]bool, name string, extra map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	if names[name] {
		return extra, dec.SkipValue()
	}
	value, err := dec.ReadValue()
	if err != nil {
		return extra, err
	}
	if extra == nil {
		extra = map[string]json.RawMessage{}
	}
	extra[name] = bytes.Clone(value)
	return extra, nil
}
