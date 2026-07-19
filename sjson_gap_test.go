package sjson

import (
	"encoding/json"
	"reflect"
	"testing"
)

type testTextKey int

func (k testTextKey) MarshalText() ([]byte, error) { return []byte("key"), nil }

type testJSONValue struct{ Value string }

func (v testJSONValue) MarshalJSON() ([]byte, error) { return []byte(`"custom:` + v.Value + `"`), nil }

func (v *testJSONValue) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	v.Value = raw
	return nil
}

func TestNullClearsPointerTargets(t *testing.T) {
	type item struct {
		Value int `json:"value"`
	}
	type target struct {
		Int   *int            `json:"int"`
		Item  *item           `json:"item"`
		Slice *[]int          `json:"slice"`
		Map   *map[string]int `json:"map"`
	}

	i := 1
	s := []int{1}
	m := map[string]int{"a": 1}
	got := target{Int: &i, Item: &item{Value: 1}, Slice: &s, Map: &m}
	if err := Unmarshal([]byte(`{"int":null,"item":null,"slice":null,"map":null}`), &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if got.Int != nil || got.Item != nil || got.Slice != nil || got.Map != nil {
		t.Fatalf("null must clear pointer fields, got %+v", got)
	}
}

func TestNumericConversionMatchesStdlib(t *testing.T) {
	cases := []struct {
		name  string
		input string
		newV  func() any
	}{
		{"fraction-to-int", `1.5`, func() any { return new(int) }},
		{"negative-to-uint", `-1`, func() any { return new(uint) }},
		{"int8-overflow", `128`, func() any { return new(int8) }},
		{"uint8-overflow", `256`, func() any { return new(uint8) }},
		{"float32-overflow", `3.5e38`, func() any { return new(float32) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.newV()
			want := tc.newV()
			gotErr := Unmarshal([]byte(tc.input), got)
			wantErr := json.Unmarshal([]byte(tc.input), want)
			if (gotErr == nil) != (wantErr == nil) {
				t.Fatalf("error mismatch: sjson=%v, encoding/json=%v", gotErr, wantErr)
			}
			if gotErr == nil && !reflect.DeepEqual(got, want) {
				t.Fatalf("value mismatch: sjson=%v, encoding/json=%v", got, want)
			}
		})
	}
}

func TestMapKeyCompatibility(t *testing.T) {
	t.Run("decode integer key", func(t *testing.T) {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("Unmarshal must return an error rather than panic: %v", recovered)
			}
		}()
		var got map[int]string
		if err := Unmarshal([]byte(`{"12":"value"}`), &got); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}
		if !reflect.DeepEqual(got, map[int]string{12: "value"}) {
			t.Fatalf("got %#v", got)
		}
	})

	t.Run("escape string key", func(t *testing.T) {
		in := map[string]int{"quote\"slash\\newline\n": 1}
		got, err := Marshal(in)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		if !json.Valid(got) {
			t.Fatalf("Marshal produced invalid JSON: %q", got)
		}
		var roundTrip map[string]int
		if err := json.Unmarshal(got, &roundTrip); err != nil {
			t.Fatalf("stdlib cannot read output: %v", err)
		}
		if !reflect.DeepEqual(roundTrip, in) {
			t.Fatalf("got %#v, want %#v", roundTrip, in)
		}
	})

	t.Run("TextMarshaler key", func(t *testing.T) {
		got, err := Marshal(map[testTextKey]string{1: "value"})
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		if string(got) != `{"key":"value"}` {
			t.Fatalf("got %s", got)
		}
	})
}

func TestCustomJSONMarshalerCompatibility(t *testing.T) {
	in := testJSONValue{Value: "value"}
	got, gotErr := Marshal(in)
	want, wantErr := json.Marshal(in)
	if (gotErr == nil) != (wantErr == nil) {
		t.Fatalf("Marshal error mismatch: sjson=%v encoding/json=%v", gotErr, wantErr)
	}
	if gotErr == nil && string(got) != string(want) {
		t.Fatalf("Marshal mismatch: sjson=%s encoding/json=%s", got, want)
	}

	var decoded testJSONValue
	if err := Unmarshal([]byte(`"decoded"`), &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded.Value != "decoded" {
		t.Fatalf("custom UnmarshalJSON was not used: %#v", decoded)
	}
}

func TestAdditionalInvalidNumberForms(t *testing.T) {
	for _, input := range []string{`01`, `-01`, `1.`, `1e`, `1e+`, `--1`, `+1`} {
		t.Run(input, func(t *testing.T) {
			var got any
			if err := Unmarshal([]byte(input), &got); err == nil {
				t.Fatalf("expected invalid JSON error for %q; got %#v", input, got)
			}
		})
	}
}

func TestRawMessageCompatibility(t *testing.T) {
	type holder struct {
		Payload json.RawMessage `json:"payload"`
	}
	input := []byte(`{"payload":{"nested":[1,true]}}`)
	var got holder
	if err := Unmarshal(input, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if string(got.Payload) != `{"nested":[1,true]}` {
		t.Fatalf("got %s", got.Payload)
	}
}
