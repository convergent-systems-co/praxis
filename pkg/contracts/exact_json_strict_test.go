package contracts

import (
	"encoding/json"
	"errors"
	"math/rand"
	"strings"
	"testing"
)

// esc turns the @ placeholder into a JSON backslash so escape sequences can
// be spelled without the source containing raw escapes.
func esc(text string) string { return strings.ReplaceAll(text, "@", string(rune(92))) }

type exactEmbedded struct {
	Shared string `json:"shared"`
}

type exactDoc struct {
	exactEmbedded
	Status string `json:"status,omitempty"`
	Inner  struct {
		A int `json:"a"`
	} `json:"inner"`
	Items   []exactItem          `json:"items"`
	ByName  map[string]int       `json:"by_name"`
	Skey    string               `json:"skey"`
	Key     string               `json:"key"`
	Ignored string               `json:"-"`
	Pointer *exactItem           `json:"pointer"`
	Free    any                  `json:"free"`
	Raw     json.RawMessage      `json:"raw"`
	Nested  map[string]exactItem `json:"nested"`
}

type exactItem struct {
	X string `json:"x"`
}

// N4: Astra's exact examples and every equivalent spelling resolve to a field
// by exact name only.
func TestUnmarshalExactJSONRefusesSemanticDuplicateKeys(t *testing.T) {
	for name, body := range map[string]string{
		"astra Status/status":                `{"Status":"undecided","status":"approved"}`,
		"astra status/STATUS":                `{"status":"undecided","STATUS":"approved"}`,
		"single non-canonical spelling":      `{"STATUS":"approved"}`,
		"nested a/A":                         `{"inner":{"a":1,"A":2}}`,
		"nested single non-canonical":        `{"inner":{"A":2}}`,
		"non-canonical inside array element": `{"items":[{"x":"1"},{"X":"2"}]}`,
		"non-canonical in pointer target":    `{"pointer":{"X":"1"}}`,
		"non-canonical in map value struct":  `{"nested":{"k":{"X":"1"}}}`,
		"embedded promoted field spelling":   `{"SHARED":"v"}`,
		"kelvin sign for k":                  "{\"" + string(rune(0x212A)) + "ey\":\"v\"}",
		"long s for s":                       "{\"" + string(rune(0x17F)) + "key\":\"v\"}",
		"escaped non-canonical spelling":     esc(`{"@u0053tatus":"approved"}`),
		"exact duplicate":                    `{"status":"a","status":"b"}`,
		"escaped exact duplicate":            esc(`{"status":"a","@u0073tatus":"b"}`),
		"exact duplicate in map":             `{"by_name":{"a":1,"a":2}}`,
		"exact duplicate under any":          `{"free":{"a":1,"a":2}}`,
		"exact duplicate under raw":          `{"raw":{"a":1,"a":2}}`,
		"exact duplicate in array of any":    `{"free":[{"a":1,"a":2}]}`,
	} {
		var out exactDoc
		err := UnmarshalExactJSON([]byte(body), &out, true)
		if !errors.Is(err, ErrAmbiguousJSON) {
			t.Fatalf("%s: ambiguous key accepted or wrongly classified: err=%v value=%+v", name, err, out)
		}
		// The same input must be refused under the lenient unknown-field policy.
		var lenient exactDoc
		if err := UnmarshalExactJSON([]byte(body), &lenient, false); !errors.Is(err, ErrAmbiguousJSON) {
			t.Fatalf("%s: lenient policy accepted ambiguity: %v", name, err)
		}
	}
}

func TestUnmarshalExactJSONAcceptsExactlyOneCompleteUnambiguousValue(t *testing.T) {
	valid := `{"shared":"s","status":"undecided","inner":{"a":1},"items":[{"x":"1"},{"x":"2"}],"by_name":{"a":1,"A":2},"skey":"s","key":"k","pointer":{"x":"p"},"free":{"A":1,"a":2},"raw":{"A":1},"nested":{"k":{"x":"1"},"K":{"x":"2"}}}` + " \n\t "
	var out exactDoc
	if err := UnmarshalExactJSON([]byte(valid), &out, true); err != nil {
		t.Fatalf("a canonical document was refused: %v", err)
	}
	if out.Status != "undecided" || out.Shared != "s" || out.Inner.A != 1 || len(out.Items) != 2 || out.ByName["A"] != 2 || out.ByName["a"] != 1 || out.Pointer == nil || out.Pointer.X != "p" || len(out.Nested) != 2 {
		t.Fatalf("canonical document decoded incorrectly: %+v", out)
	}
	// Distinct-case keys are distinct keys for maps and untyped values.
	var free map[string]any
	if err := UnmarshalExactJSON([]byte(`{"a":1,"A":2}`), &free, false); err != nil || len(free) != 2 {
		t.Fatalf("distinct-case map keys were refused: %v %v", err, free)
	}
	// Non-object roots, null and scalars remain valid values.
	for _, body := range []string{`null`, `[]`, `[{"x":"1"}]`, `"text"`, `12`, `true`} {
		var any any
		if err := UnmarshalExactJSON([]byte(body), &any, false); err != nil {
			t.Fatalf("%s: valid value refused: %v", body, err)
		}
	}
	var nullRoot exactDoc
	if err := UnmarshalExactJSON([]byte(`null`), &nullRoot, true); err != nil {
		t.Fatalf("null root refused: %v", err)
	}
}

func TestUnmarshalExactJSONRefusesMalformedAndUnboundedInput(t *testing.T) {
	deep := strings.Repeat("[", MaxExactJSONDepth+2) + strings.Repeat("]", MaxExactJSONDepth+2)
	deepObjects := strings.Repeat(`{"a":`, MaxExactJSONDepth+2) + `1` + strings.Repeat("}", MaxExactJSONDepth+2)
	for name, body := range map[string][]byte{
		"trailing garbage":       []byte(`{"status":"a"} trailing`),
		"second object":          []byte(`{"status":"a"} {"status":"b"}`),
		"trailing comma value":   []byte(`{"status":"a"},`),
		"trailing scalar":        []byte(`{"status":"a"} 1`),
		"empty":                  nil,
		"whitespace only":        []byte(" \n"),
		"truncated":              []byte(`{"status":"a"`),
		"unknown field":          []byte(`{"status":"a","extra":1}`),
		"bom":                    append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"status":"a"}`)...),
		"invalid utf8 in string": append(append([]byte(`{"status":"`), 0xff, 0xfe), []byte(`"}`)...),
		"invalid utf8 in key":    append(append([]byte(`{"st`), 0xc3), []byte(`":"a"}`)...),
		"lone high surrogate":    []byte(esc(`{"status":"@ud800"}`)),
		"lone low surrogate":     []byte(esc(`{"status":"@udc00"}`)),
		"high then non-low":      []byte(esc(`{"status":"@ud800@u0041"}`)),
		"depth bomb arrays":      []byte(deep),
		"depth bomb objects":     []byte(deepObjects),
		"oversized":              []byte(`{"status":"` + strings.Repeat("x", MaxGovernedArtifactBytes) + `"}`),
		"non-string key":         []byte(`{1:"a"}`),
	} {
		var out exactDoc
		if err := UnmarshalExactJSON(body, &out, true); err == nil {
			t.Fatalf("%s: malformed or unbounded input was accepted", name)
		}
	}
	// A well-formed surrogate pair and a raw U+FFFD are ordinary valid text.
	var ok exactDoc
	if err := UnmarshalExactJSON([]byte(esc(`{"status":"@ud83d@ude00 @ufffd"}`)), &ok, true); err != nil {
		t.Fatalf("a valid surrogate pair was refused: %v", err)
	}
	// Exactly at the bound is accepted; one byte over is refused.
	pad := MaxGovernedArtifactBytes - len(`{"status":""}`)
	var atBound exactDoc
	if err := UnmarshalExactJSON([]byte(`{"status":"`+strings.Repeat("x", pad)+`"}`), &atBound, true); err != nil {
		t.Fatalf("input exactly at the bound was refused: %v", err)
	}
	if err := UnmarshalExactJSON([]byte(`{"status":"`+strings.Repeat("x", pad+1)+`"}`), &atBound, true); err == nil {
		t.Fatal("input one byte over the bound was accepted")
	}
}

// Randomized differential check: for every key-case mutation of a valid
// document, the exact parser refuses precisely when the document is not the
// canonical spelling, and encoding/json would have silently accepted that
// non-canonical spelling (which is the ambiguity being closed).
func TestUnmarshalExactJSONDifferentialAgainstEncodingJSON(t *testing.T) {
	type doc struct {
		Status string `json:"status"`
		Inner  struct {
			Alpha int `json:"alpha"`
			Beta  int `json:"beta"`
		} `json:"inner"`
		Items []struct {
			Value string `json:"value"`
		} `json:"items"`
	}
	canonical := []string{"status", "inner", "alpha", "beta", "items", "value"}
	random := rand.New(rand.NewSource(20260920))
	mutate := func(word string) string {
		b := []byte(word)
		for i := range b {
			if random.Intn(3) == 0 {
				b[i] ^= 0x20
			}
		}
		return string(b)
	}
	sawNonCanonical := 0
	for i := 0; i < 400; i++ {
		spelling := map[string]string{}
		mutated := false
		for _, word := range canonical {
			spelling[word] = mutate(word)
			if spelling[word] != word {
				mutated = true
			}
		}
		body := `{"` + spelling["status"] + `":"undecided","` + spelling["inner"] + `":{"` + spelling["alpha"] + `":1,"` + spelling["beta"] + `":2},"` + spelling["items"] + `":[{"` + spelling["value"] + `":"v"}]}`
		var standard, exact doc
		standardErr := json.Unmarshal([]byte(body), &standard)
		exactErr := UnmarshalExactJSON([]byte(body), &exact, true)
		if !mutated {
			if exactErr != nil {
				t.Fatalf("canonical document refused: %s: %v", body, exactErr)
			}
			continue
		}
		if standardErr != nil || standard.Status != "undecided" || standard.Inner.Alpha != 1 || len(standard.Items) != 1 {
			t.Fatalf("differential premise broken: encoding/json did not accept %s: %v", body, standardErr)
		}
		sawNonCanonical++
		if exactErr == nil {
			t.Fatalf("non-canonical spelling accepted by the exact parser but silently matched by encoding/json: %s", body)
		}
	}
	if sawNonCanonical < 100 {
		t.Fatalf("differential check exercised too few mutations: %d", sawNonCanonical)
	}
	// Two spellings of one field: encoding/json keeps the last; the exact
	// parser refuses.
	var standard doc
	twice := `{"status":"undecided","STATUS":"approved"}`
	if err := json.Unmarshal([]byte(twice), &standard); err != nil || standard.Status != "approved" {
		t.Fatalf("premise: encoding/json resolves both keys to one field, last wins: %v %q", err, standard.Status)
	}
	var exact doc
	if err := UnmarshalExactJSON([]byte(twice), &exact, true); err == nil {
		t.Fatal("the exact parser accepted two spellings of one field")
	}
}

// The depth bound is proven against a document that is valid JSON and decodes
// cleanly into an untyped destination (so no other check can refuse it).
func TestUnmarshalExactJSONNestingBoundRefusesOtherwiseValidDocuments(t *testing.T) {
	ok := strings.Repeat(`{"a":`, MaxExactJSONDepth-2) + `1` + strings.Repeat("}", MaxExactJSONDepth-2)
	var sink any
	if err := UnmarshalExactJSON([]byte(ok), &sink, false); err != nil {
		t.Fatalf("control: a document within the bound was refused: %v", err)
	}
	tooDeep := strings.Repeat(`{"a":`, MaxExactJSONDepth+5) + `1` + strings.Repeat("}", MaxExactJSONDepth+5)
	if err := UnmarshalExactJSON([]byte(tooDeep), &sink, false); !errors.Is(err, ErrAmbiguousJSON) {
		t.Fatalf("a document nested beyond the bound was accepted: %v", err)
	}
}
