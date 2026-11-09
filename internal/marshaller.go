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

// Member is a member of a body that the body writes itself, straight into
// the request.
type Member struct {
	Name  string
	Value any
}

// MemberSplitter is a body with extra fields whose largest members are
// written straight into the request, instead of once into the document the
// extra fields are merged into and once more into the request.
type MemberSplitter interface {
	// SplitMembers returns the body without the members it writes itself,
	// left empty, and those members as the body writes them (not those it
	// omits). The members the extra fields name, for which named is true,
	// stay in the body, for the merge to reach them.
	SplitMembers(named func(name string) bool) (rest any, members []Member)
}

type JSONMarshaller struct{}

// Marshal writes value. A body with extra fields is written sorted and
// merged with them as by mergePatch over the marshalled body; the members of
// a MemberSplitter the extra fields do not name are written once, into the
// request.
func (jm *JSONMarshaller) Marshal(value any) ([]byte, error) {
	// Check if the value implements the GetExtraFields interface
	getExtraFieldsBody, ok := value.(interface {
		GetExtraFields() map[string]any
	})
	if !ok {
		return jsonv2.Marshal(value, EncodeOptions)
	}
	extraFields := getExtraFieldsBody.GetExtraFields()
	if len(extraFields) == 0 {
		return jsonv2.Marshal(value, EncodeOptions)
	}
	rest, members := value, []Member(nil)
	if splitter, ok := value.(MemberSplitter); ok {
		rest, members = splitter.SplitMembers(func(name string) bool {
			_, named := extraFields[name]
			return named
		})
	}
	originalBytes, err := jsonv2.Marshal(rest, EncodeOptions)
	if err != nil {
		return nil, bodyError(value, members, err)
	}
	patchBytes, err := jsonv2.Marshal(extraFields, EncodeOptions)
	if err != nil {
		if err := bodyError(value, members, nil); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("Marshal extraFields(%+v) err: %w", extraFields, err)
	}
	root, err := mergePatch(originalBytes, patchBytes)
	if err != nil {
		return nil, fmt.Errorf("MergePatch originalBytes(%s) patchBytes(%s) err: %w", originalBytes, patchBytes, err)
	}
	for _, m := range members {
		root.obj[m.Name] = &mergeNode{kind: 'm', value: m.Value}
	}
	finalBytes, err := jsonv2.Marshal(root, EncodeOptions)
	if err != nil {
		return nil, bodyError(value, members, err)
	}
	return finalBytes, nil
}

// bodyError is the error of marshalling the whole body, which comes before
// those of its extra fields: with members written apart, err could name
// another member first, or a member through the merge. Only errors pay for
// marshalling the body again.
func bodyError(value any, members []Member, err error) error {
	if len(members) > 0 {
		if _, bodyErr := jsonv2.Marshal(value, EncodeOptions); bodyErr != nil {
			return bodyErr
		}
	}
	return err
}

// mergePatch applies the JSON merge patch patch (RFC 7386) to the JSON
// object doc, with the result github.com/evanphx/json-patch v0.5.2 gave for
// a document and a patch that are objects: the members of every object it
// opens (the document, the objects merged, the objects and arrays of the
// patch, from which nulls are pruned) sorted by name, every other value as
// it was, written with the options of encoding/json by the root returned
// (MarshalJSONTo). Values are read as tokens; only the objects the patch
// reaches are opened.
func mergePatch(doc, patch []byte) (*mergeNode, error) {
	root := &mergeNode{raw: doc}
	if !root.intoDoc() {
		return nil, fmt.Errorf("merge patch: the document is not a JSON object")
	}
	p := &mergeNode{raw: patch}
	if !p.intoDoc() {
		return nil, fmt.Errorf("merge patch: the patch is not a JSON object")
	}
	mergeDocs(root.obj, p.obj)
	return root, nil
}

// MarshalJSONTo writes the merged document into the encoder of the request.
func (n *mergeNode) MarshalJSONTo(enc *jsontext.Encoder) error {
	return n.write(enc)
}

// mergeNode is a JSON value as the merge sees it: verbatim until the merge
// opens it as an object (obj, a nil member is null) or an array (arr, a nil
// element is null). A member a body writes itself (kind 'm') is added to the
// root after the merge: its value is marshalled when the root is written.
type mergeNode struct {
	raw   jsontext.Value
	kind  byte // 0 verbatim, '{' object, '[' array, 'm' member written by the body
	obj   map[string]*mergeNode
	arr   []*mergeNode
	value any
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
	case n.kind == 'm':
		return jsonv2.MarshalEncode(enc, n.value)
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
