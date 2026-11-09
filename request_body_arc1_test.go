package openai_test

// The request body v0.1.6-arc.1 wrote, kept verbatim as the reference of
// TestRequestBodyMatchesArc1: the whole request marshalled, then its extra
// fields merged by MergePatch into a second document. Test code only.

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"fmt"
	"slices"

	openai "github.com/meguminnnnnnnnn/go-openai/internal"
)

func arc1Marshal(value any) ([]byte, error) {
	originalBytes, err := jsonv2.Marshal(value, openai.EncodeOptions)
	if err != nil {
		return nil, err
	}
	getExtraFieldsBody, ok := value.(interface {
		GetExtraFields() map[string]any
	})
	if !ok {
		return originalBytes, nil
	}
	extraFields := getExtraFieldsBody.GetExtraFields()
	if len(extraFields) == 0 {
		return originalBytes, nil
	}
	patchBytes, err := jsonv2.Marshal(extraFields, openai.EncodeOptions)
	if err != nil {
		return nil, fmt.Errorf("Marshal extraFields(%+v) err: %w", extraFields, err)
	}
	finalBytes, err := arc1MergePatch(originalBytes, patchBytes)
	if err != nil {
		return nil, fmt.Errorf("MergePatch originalBytes(%s) patchBytes(%s) err: %w", originalBytes, patchBytes, err)
	}
	return finalBytes, nil
}

func arc1MergePatch(doc, patch []byte) ([]byte, error) {
	root := &arc1Node{raw: doc}
	if !root.intoDoc() {
		return nil, fmt.Errorf("merge patch: the document is not a JSON object")
	}
	p := &arc1Node{raw: patch}
	if !p.intoDoc() {
		return nil, fmt.Errorf("merge patch: the patch is not a JSON object")
	}
	arc1MergeDocs(root.obj, p.obj)
	var out bytes.Buffer
	out.Grow(len(doc) + len(patch))
	enc := jsontext.NewEncoder(&out, openai.EncodeOptions)
	if err := root.write(enc); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(out.Bytes(), []byte("\n")), nil
}

type arc1Node struct {
	raw  jsontext.Value
	kind byte
	obj  map[string]*arc1Node
	arr  []*arc1Node
}

func (n *arc1Node) intoDoc() bool {
	if n.kind == '{' {
		return true
	}
	if n.kind != 0 || n.raw.Kind() != '{' {
		return false
	}
	obj := map[string]*arc1Node{}
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
		obj[name] = arc1NodeOf(value)
	}
	n.kind, n.obj = '{', obj
	return true
}

func (n *arc1Node) intoAry() bool {
	if n.kind == '[' {
		return true
	}
	if n.kind != 0 || n.raw.Kind() != '[' {
		return false
	}
	arr := []*arc1Node{}
	dec := jsontext.NewDecoder(bytes.NewBuffer(n.raw), jsontext.AllowDuplicateNames(true), jsontext.AllowInvalidUTF8(true))
	if _, err := dec.ReadToken(); err != nil {
		return false
	}
	for dec.PeekKind() != ']' {
		value, err := dec.ReadValue()
		if err != nil {
			return false
		}
		arr = append(arr, arc1NodeOf(value))
	}
	n.kind, n.arr = '[', arr
	return true
}

func arc1NodeOf(value jsontext.Value) *arc1Node {
	if value.Kind() == 'n' {
		return nil
	}
	return &arc1Node{raw: value}
}

func arc1MergeDocs(doc, patch map[string]*arc1Node) {
	for k, v := range patch {
		if v == nil {
			delete(doc, k)
			continue
		}
		if cur, ok := doc[k]; !ok || cur == nil {
			arc1PruneNulls(v)
			doc[k] = v
		} else {
			doc[k] = arc1Merge(cur, v)
		}
	}
}

func arc1Merge(cur, patch *arc1Node) *arc1Node {
	if !cur.intoDoc() {
		arc1PruneNulls(patch)
		return patch
	}
	if !patch.intoDoc() {
		return patch
	}
	arc1MergeDocs(cur.obj, patch.obj)
	return cur
}

func arc1PruneNulls(n *arc1Node) {
	if n.intoDoc() {
		for k, v := range n.obj {
			if v == nil {
				delete(n.obj, k)
			} else {
				arc1PruneNulls(v)
			}
		}
	} else if n.intoAry() {
		kept := []*arc1Node{}
		for _, v := range n.arr {
			if v != nil {
				arc1PruneNulls(v)
				kept = append(kept, v)
			}
		}
		n.arr = kept
	}
}

func (n *arc1Node) write(enc *jsontext.Encoder) error {
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
