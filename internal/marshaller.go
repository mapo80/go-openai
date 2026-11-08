package openai

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"fmt"
	"slices"
)

// EncodeOptions are the options of the encodings the SDK makes directly
// with encoding/json/v2: those of encoding/json, so that the bytes sent are
// the ones the SDK always sent.
var EncodeOptions = json.DefaultOptionsV1()

type Marshaller interface {
	Marshal(value any) ([]byte, error)
}

type JSONMarshaller struct{}

func (jm *JSONMarshaller) Marshal(value any) ([]byte, error) {
	originalBytes, err := jsonv2.Marshal(value, EncodeOptions)
	if err != nil {
		return nil, err
	}
	// Check if the value implements the GetExtraFields interface
	getExtraFieldsBody, ok := value.(interface {
		GetExtraFields() map[string]any
	})
	if !ok {
		// If not, return the original bytes
		return originalBytes, nil
	}
	extraFields := getExtraFieldsBody.GetExtraFields()
	if len(extraFields) == 0 {
		// If there are no extra fields, return the original bytes
		return originalBytes, nil
	}
	patchBytes, err := jsonv2.Marshal(extraFields, EncodeOptions)
	if err != nil {
		return nil, fmt.Errorf("Marshal extraFields(%+v) err: %w", extraFields, err)
	}
	finalBytes, err := MergePatch(originalBytes, patchBytes)
	if err != nil {
		return nil, fmt.Errorf("MergePatch originalBytes(%s) patchBytes(%s) err: %w", originalBytes, patchBytes, err)
	}
	return finalBytes, nil
}

// MergePatch applies the JSON merge patch patch (RFC 7386) to the JSON
// object doc, with the result github.com/evanphx/json-patch v0.5.2 gave for
// a document and a patch that are objects: the members of every object it
// opens (the document, the objects merged, the objects and arrays of the
// patch, from which nulls are pruned) sorted by name, every other value as
// it was, written with the options of encoding/json. Values are read as
// tokens; only the objects the patch reaches are opened.
func MergePatch(doc, patch []byte) ([]byte, error) {
	root := &mergeNode{raw: doc}
	if !root.intoDoc() {
		return nil, fmt.Errorf("merge patch: the document is not a JSON object")
	}
	p := &mergeNode{raw: patch}
	if !p.intoDoc() {
		return nil, fmt.Errorf("merge patch: the patch is not a JSON object")
	}
	mergeDocs(root.obj, p.obj)
	var out bytes.Buffer
	out.Grow(len(doc) + len(patch))
	enc := jsontext.NewEncoder(&out, EncodeOptions)
	if err := root.write(enc); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(out.Bytes(), []byte("\n")), nil
}

// mergeNode is a JSON value as the merge sees it: verbatim until the merge
// opens it as an object (obj, a nil member is null) or an array (arr, a nil
// element is null).
type mergeNode struct {
	raw  jsontext.Value
	kind byte // 0 verbatim, '{' object, '[' array
	obj  map[string]*mergeNode
	arr  []*mergeNode
}

func (n *mergeNode) intoDoc() bool {
	if n.kind == '{' {
		return true
	}
	if n.kind != 0 || n.raw.Kind() != '{' {
		return false
	}
	obj := map[string]*mergeNode{}
	dec := jsontext.NewDecoder(bytes.NewBuffer(n.raw), jsontext.AllowDuplicateNames(true), jsontext.AllowInvalidUTF8(true))
	if _, err := dec.ReadToken(); err != nil {
		return false
	}
	for dec.PeekKind() != '}' {
		tok, err := dec.ReadToken()
		if err != nil {
			return false
		}
		name := tok.String()
		value, err := dec.ReadValue()
		if err != nil {
			return false
		}
		obj[name] = node(value)
	}
	n.kind, n.obj = '{', obj
	return true
}

func (n *mergeNode) intoAry() bool {
	if n.kind == '[' {
		return true
	}
	if n.kind != 0 || n.raw.Kind() != '[' {
		return false
	}
	arr := []*mergeNode{}
	dec := jsontext.NewDecoder(bytes.NewBuffer(n.raw), jsontext.AllowDuplicateNames(true), jsontext.AllowInvalidUTF8(true))
	if _, err := dec.ReadToken(); err != nil {
		return false
	}
	for dec.PeekKind() != ']' {
		value, err := dec.ReadValue()
		if err != nil {
			return false
		}
		arr = append(arr, node(value))
	}
	n.kind, n.arr = '[', arr
	return true
}

// node is the node of value, nil for null. The decoders read the bytes of
// the document and of the patch in place (bytes.Buffer): value is a slice of
// them, owned by the merge.
func node(value jsontext.Value) *mergeNode {
	if value.Kind() == 'n' {
		return nil
	}
	return &mergeNode{raw: value}
}

func mergeDocs(doc, patch map[string]*mergeNode) {
	for k, v := range patch {
		if v == nil {
			delete(doc, k)
			continue
		}
		if cur, ok := doc[k]; !ok || cur == nil {
			pruneNulls(v)
			doc[k] = v
		} else {
			doc[k] = merge(cur, v)
		}
	}
}

func merge(cur, patch *mergeNode) *mergeNode {
	if !cur.intoDoc() {
		pruneNulls(patch)
		return patch
	}
	if !patch.intoDoc() {
		return patch
	}
	mergeDocs(cur.obj, patch.obj)
	return cur
}

func pruneNulls(n *mergeNode) {
	if n.intoDoc() {
		for k, v := range n.obj {
			if v == nil {
				delete(n.obj, k)
			} else {
				pruneNulls(v)
			}
		}
	} else if n.intoAry() {
		kept := []*mergeNode{}
		for _, v := range n.arr {
			if v != nil {
				pruneNulls(v)
				kept = append(kept, v)
			}
		}
		n.arr = kept
	}
}

func (n *mergeNode) write(enc *jsontext.Encoder) error {
	switch {
	case n == nil:
		return enc.WriteToken(jsontext.Null)
	case n.kind == '{':
		if err := enc.WriteToken(jsontext.BeginObject); err != nil {
			return err
		}
		names := make([]string, 0, len(n.obj))
		for k := range n.obj {
			names = append(names, k)
		}
		slices.Sort(names)
		for _, k := range names {
			if err := enc.WriteToken(jsontext.String(k)); err != nil {
				return err
			}
			if err := n.obj[k].write(enc); err != nil {
				return err
			}
		}
		return enc.WriteToken(jsontext.EndObject)
	case n.kind == '[':
		if err := enc.WriteToken(jsontext.BeginArray); err != nil {
			return err
		}
		for _, v := range n.arr {
			if err := v.write(enc); err != nil {
				return err
			}
		}
		return enc.WriteToken(jsontext.EndArray)
	}
	return enc.WriteValue(n.raw)
}
