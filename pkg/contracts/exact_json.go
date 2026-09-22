package contracts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"unicode/utf8"
)

// ErrAmbiguousJSON is returned for safety-bearing evidence that is not
// exactly one complete, unambiguous JSON value.
var ErrAmbiguousJSON = errors.New("safety-bearing JSON must be exactly one complete unambiguous value")

// MaxExactJSONDepth bounds nesting so a hostile document cannot exhaust the
// parser's recursion.
const MaxExactJSONDepth = 64

// UnmarshalExactJSON decodes safety-bearing evidence. A digest authenticates
// exact bytes; it does not make malformed or ambiguous bytes semantically
// valid, so this parser additionally requires that the input is bounded, valid
// UTF-8 (Go would otherwise substitute U+FFFD silently), exactly one JSON value
// with no trailing content, and unambiguous at every depth.
//
// Ambiguity is decided against the DESTINATION type, not the text of the keys.
// encoding/json matches an object key to a struct field case-insensitively, so
// {"status":"undecided","STATUS":"approved"} contains no textually duplicate
// key yet resolves to one field twice, last value winning. The strict walk
// resolves every key to a field by its exact name and refuses a key that only
// matches a field by case folding; keys resolving to the same field are
// refused; map destinations refuse exact duplicates (distinct-case map keys
// remain distinct keys). When disallowUnknown is set, unknown fields are
// refused.
func UnmarshalExactJSON(data []byte, out any, disallowUnknown bool) error {
	if len(data) == 0 || len(data) > MaxGovernedArtifactBytes {
		return fmt.Errorf("%w: input is empty or exceeds %d bytes", ErrAmbiguousJSON, MaxGovernedArtifactBytes)
	}
	if !utf8.Valid(data) {
		return fmt.Errorf("%w: input is not valid UTF-8", ErrAmbiguousJSON)
	}
	if err := rejectUnpairedSurrogateEscapes(data); err != nil {
		return err
	}
	var destination reflect.Type
	if out != nil {
		destination = reflect.TypeOf(out)
		for destination != nil && destination.Kind() == reflect.Pointer {
			destination = destination.Elem()
		}
	}
	walker := json.NewDecoder(bytes.NewReader(data))
	if err := walkExact(walker, destination, 0); err != nil {
		return err
	}
	if _, err := walker.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing content after the first value", ErrAmbiguousJSON)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if disallowUnknown {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing content after the first value", ErrAmbiguousJSON)
	}
	return nil
}

// rejectUnpairedSurrogateEscapes refuses a \uD800-style escape that is not a
// well-formed surrogate pair; encoding/json would replace it with U+FFFD, so
// two different byte strings would decode to the same value.
func rejectUnpairedSurrogateEscapes(data []byte) error {
	inString := false
	for i := 0; i < len(data); i++ {
		c := data[i]
		if !inString {
			if c == '"' {
				inString = true
			}
			continue
		}
		switch c {
		case '"':
			inString = false
		case '\\':
			if i+1 >= len(data) {
				return nil
			}
			if data[i+1] != 'u' {
				i++
				continue
			}
			unit, ok := hexUnit(data, i+2)
			if !ok {
				return nil // malformed escape: reported by the decoder
			}
			i += 5
			switch {
			case unit >= 0xD800 && unit < 0xDC00:
				if i+6 < len(data) && data[i+1] == '\\' && data[i+2] == 'u' {
					if low, ok := hexUnit(data, i+3); ok && low >= 0xDC00 && low < 0xE000 {
						i += 6
						continue
					}
				}
				return fmt.Errorf("%w: unpaired surrogate escape", ErrAmbiguousJSON)
			case unit >= 0xDC00 && unit < 0xE000:
				return fmt.Errorf("%w: unpaired surrogate escape", ErrAmbiguousJSON)
			}
		}
	}
	return nil
}

func hexUnit(data []byte, at int) (rune, bool) {
	if at+4 > len(data) {
		return 0, false
	}
	var value rune
	for _, c := range data[at : at+4] {
		switch {
		case c >= '0' && c <= '9':
			value = value<<4 | rune(c-'0')
		case c >= 'a' && c <= 'f':
			value = value<<4 | rune(c-'a'+10)
		case c >= 'A' && c <= 'F':
			value = value<<4 | rune(c-'A'+10)
		default:
			return 0, false
		}
	}
	return value, true
}

// walkExact consumes exactly one value from the token stream while checking it
// against the destination type (nil means the shape is unknown: objects are
// then only checked for exact duplicate keys). Malformed syntax is reported by
// the token stream; trailing values by the caller.
func walkExact(decoder *json.Decoder, destination reflect.Type, depth int) error {
	if depth > MaxExactJSONDepth {
		return fmt.Errorf("%w: nesting exceeds %d levels", ErrAmbiguousJSON, MaxExactJSONDepth)
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	destination = shapeOf(destination)
	switch delim {
	case '{':
		var fields *exactFields
		var elem reflect.Type
		if destination != nil {
			switch destination.Kind() {
			case reflect.Struct:
				fields = exactFieldsOf(destination)
			case reflect.Map:
				elem = destination.Elem()
			}
		}
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("%w: non-string object key", ErrAmbiguousJSON)
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("%w: duplicate object key %q", ErrAmbiguousJSON, key)
			}
			seen[key] = struct{}{}
			var valueType reflect.Type
			switch {
			case fields != nil:
				if field, exact := fields.exact[key]; exact {
					valueType = field
				} else if canonical, folded := fields.folded(key); folded {
					return fmt.Errorf("%w: object key %q is a non-canonical spelling of field %q (encoding/json would silently match it)", ErrAmbiguousJSON, key, canonical)
				}
			case elem != nil:
				valueType = elem
			}
			if err := walkExact(decoder, valueType, depth+1); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err
	case '[':
		var elem reflect.Type
		if destination != nil && (destination.Kind() == reflect.Slice || destination.Kind() == reflect.Array) {
			elem = destination.Elem()
		}
		for decoder.More() {
			if err := walkExact(decoder, elem, depth+1); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err
	}
	return nil
}

var (
	unmarshalerType = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()
	rawMessageType  = reflect.TypeOf(json.RawMessage(nil))
)

// shapeOf strips pointers and returns nil for destinations whose JSON shape is
// not described by their Go type (interfaces, raw messages and custom
// unmarshalers), which are then checked structurally only.
func shapeOf(t reflect.Type) reflect.Type {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t == rawMessageType || t.Kind() == reflect.Interface {
		return nil
	}
	if t.Implements(unmarshalerType) || reflect.PointerTo(t).Implements(unmarshalerType) {
		return nil
	}
	return t
}

// exactFields is the exact-name field index of one struct type, mirroring the
// name resolution encoding/json applies (tags, "-", embedded promotion).
type exactFields struct {
	exact map[string]reflect.Type
	names []string
}

// folded reports whether key matches some field only by case folding.
func (f *exactFields) folded(key string) (string, bool) {
	for _, name := range f.names {
		if strings.EqualFold(key, name) {
			return name, true
		}
	}
	return "", false
}

func exactFieldsOf(t reflect.Type) *exactFields {
	fields := &exactFields{exact: map[string]reflect.Type{}}
	type candidate struct {
		name  string
		typ   reflect.Type
		depth int
	}
	var collected []candidate
	visited := map[reflect.Type]bool{}
	var collect func(t reflect.Type, depth int)
	collect = func(t reflect.Type, depth int) {
		if visited[t] {
			return
		}
		visited[t] = true
		defer func() { visited[t] = false }()
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			tag := field.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name, _, _ := strings.Cut(tag, ",")
			if field.Anonymous {
				ft := field.Type
				if ft.Kind() == reflect.Pointer {
					ft = ft.Elem()
				}
				if name == "" && ft.Kind() == reflect.Struct {
					collect(ft, depth+1)
					continue
				}
			}
			if !field.IsExported() {
				continue
			}
			if name == "" {
				name = field.Name
			}
			collected = append(collected, candidate{name: name, typ: field.Type, depth: depth})
		}
	}
	collect(t, 0)
	// Shallower fields win; equal-depth conflicts are unresolvable, so the
	// name is kept with an unknown shape (checked structurally only).
	best := map[string]candidate{}
	conflict := map[string]bool{}
	for _, c := range collected {
		current, ok := best[c.name]
		switch {
		case !ok || c.depth < current.depth:
			best[c.name] = c
			conflict[c.name] = false
		case c.depth == current.depth:
			conflict[c.name] = true
		}
	}
	for name, c := range best {
		if conflict[name] {
			fields.exact[name] = nil
		} else {
			fields.exact[name] = c.typ
		}
		fields.names = append(fields.names, name)
	}
	return fields
}
