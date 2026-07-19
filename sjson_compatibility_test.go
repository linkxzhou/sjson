package sjson

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

// =====================================================
// 功能完整性测试 - 与 encoding/json 行为一致性验证
// =====================================================

// --- 1. 数值边界测试 ---

func TestIntBoundary(t *testing.T) {
	cases := []struct {
		json string
		val  int64
	}{
		{"0", 0},
		{"-1", -1},
		{"2147483647", math.MaxInt32},
		{"-2147483648", math.MinInt32},
		{"9223372036854775807", math.MaxInt64},
		{"-9223372036854775808", math.MinInt64},
	}
	for _, c := range cases {
		var got int64
		if err := Unmarshal([]byte(c.json), &got); err != nil {
			t.Errorf("Unmarshal(%s) error: %v", c.json, err)
			continue
		}
		if got != c.val {
			t.Errorf("Unmarshal(%s) = %d, want %d", c.json, got, c.val)
		}
	}
}

func TestUintBoundary(t *testing.T) {
	cases := []struct {
		json string
		val  uint64
	}{
		{"0", 0},
		{"18446744073709551615", math.MaxUint64},
	}
	for _, c := range cases {
		var got uint64
		if err := Unmarshal([]byte(c.json), &got); err != nil {
			t.Errorf("Unmarshal(%s) error: %v", c.json, err)
			continue
		}
		if got != c.val {
			t.Errorf("Unmarshal(%s) = %d, want %d", c.json, got, c.val)
		}
	}
}

func TestFloatPrecision(t *testing.T) {
	cases := []struct {
		json string
		val  float64
	}{
		{"3.141592653589793", 3.141592653589793},
		{"1e10", 1e10},
		{"1e-10", 1e-10},
		{"-1.234e+5", -1.234e+5},
		{"0.0", 0.0},
		{"1.7976931348623157e+308", math.MaxFloat64},
		{"4.9e-324", math.SmallestNonzeroFloat64},
	}
	for _, c := range cases {
		var got float64
		if err := Unmarshal([]byte(c.json), &got); err != nil {
			t.Errorf("Unmarshal(%s) error: %v", c.json, err)
			continue
		}
		if got != c.val {
			t.Errorf("Unmarshal(%s) = %v, want %v", c.json, got, c.val)
		}
	}
}

func TestFloat32Decode(t *testing.T) {
	var got float32
	if err := Unmarshal([]byte("3.14"), &got); err != nil {
		t.Fatalf("Unmarshal float32 error: %v", err)
	}
	if got != float32(3.14) {
		t.Errorf("Unmarshal float32 = %v, want %v", got, float32(3.14))
	}
}

// --- 2. 各种整数类型测试 ---

func TestAllIntTypes(t *testing.T) {
	type IntTypes struct {
		Int8   int8   `json:"i8"`
		Int16  int16  `json:"i16"`
		Int32  int32  `json:"i32"`
		Int64  int64  `json:"i64"`
		Uint8  uint8  `json:"u8"`
		Uint16 uint16 `json:"u16"`
		Uint32 uint32 `json:"u32"`
		Uint64 uint64 `json:"u64"`
	}
	input := `{"i8":127,"i16":32767,"i32":2147483647,"i64":9223372036854775807,"u8":255,"u16":65535,"u32":4294967295,"u64":18446744073709551615}`

	var got IntTypes
	if err := Unmarshal([]byte(input), &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	var want IntTypes
	if err := json.Unmarshal([]byte(input), &want); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// --- 3. 嵌套结构体测试 ---

func TestNestedStruct(t *testing.T) {
	type Address struct {
		City    string `json:"city"`
		ZipCode string `json:"zip_code"`
	}
	type Person struct {
		Name    string  `json:"name"`
		Age     int     `json:"age"`
		Address Address `json:"address"`
	}

	input := `{"name":"Alice","age":30,"address":{"city":"Beijing","zip_code":"100000"}}`

	var got, want Person
	if err := Unmarshal([]byte(input), &got); err != nil {
		t.Fatalf("sjson.Unmarshal error: %v", err)
	}
	if err := json.Unmarshal([]byte(input), &want); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestPointerStruct(t *testing.T) {
	type Inner struct {
		Value int `json:"value"`
	}
	type Outer struct {
		Inner *Inner `json:"inner"`
	}

	// 非nil指针
	input := `{"inner":{"value":42}}`
	var got, want Outer
	if err := Unmarshal([]byte(input), &got); err != nil {
		t.Fatalf("sjson.Unmarshal error: %v", err)
	}
	if err := json.Unmarshal([]byte(input), &want); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if got.Inner == nil || got.Inner.Value != 42 {
		t.Errorf("got %+v, want inner.value=42", got)
	}

	// nil指针 (null)
	input2 := `{"inner":null}`
	var got2, want2 Outer
	if err := Unmarshal([]byte(input2), &got2); err != nil {
		t.Fatalf("sjson.Unmarshal null error: %v", err)
	}
	if err := json.Unmarshal([]byte(input2), &want2); err != nil {
		t.Fatalf("json.Unmarshal null error: %v", err)
	}
	if got2.Inner != nil {
		t.Errorf("got inner=%v, want nil", got2.Inner)
	}
}

// --- 4. 空值与默认值测试 ---

func TestNullToVariousTypes(t *testing.T) {
	type T struct {
		S    string  `json:"s"`
		I    int     `json:"i"`
		F    float64 `json:"f"`
		B    bool    `json:"b"`
		Slc  []int   `json:"slc"`
		Map  map[string]int `json:"map"`
		Ptr  *int    `json:"ptr"`
	}

	input := `{"s":"init","i":42,"f":3.14,"b":true,"slc":[1],"map":{"a":1},"ptr":null}`
	var got T
	if err := Unmarshal([]byte(input), &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// null should set ptr to nil
	if got.Ptr != nil {
		t.Errorf("ptr should be nil, got %v", got.Ptr)
	}

	// Other fields should be properly decoded
	if got.S != "init" || got.I != 42 || got.F != 3.14 || got.B != true {
		t.Errorf("fields not decoded correctly: %+v", got)
	}
}

func TestMissingFieldsKeepZero(t *testing.T) {
	type T struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	input := `{"name":"Bob"}`
	var got T
	if err := Unmarshal([]byte(input), &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if got.Name != "Bob" {
		t.Errorf("Name = %q, want %q", got.Name, "Bob")
	}
	if got.Age != 0 {
		t.Errorf("Age = %d, want 0 (zero value)", got.Age)
	}
}

// --- 5. JSON 标签测试 ---

func TestJsonTagOmitEmpty(t *testing.T) {
	type T struct {
		Name string `json:"name"`
		Age  int    `json:"age,omitempty"`
		City string `json:"city,omitempty"`
	}

	t.Run("with omitempty", func(t *testing.T) {
		v := T{Name: "Alice", Age: 0, City: "NYC"}
		got, err := Marshal(v)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		want, _ := json.Marshal(v)
		if string(got) != string(want) {
			t.Errorf("got %s, want %s", got, want)
		}
	})
}

func TestJsonTagSkipField(t *testing.T) {
	type T struct {
		Name string `json:"name"`
		Hash string `json:"-"`
	}

	v := T{Name: "Alice", Hash: "secret"}
	got, err := Marshal(v)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	want, _ := json.Marshal(v)
	if string(got) != string(want) {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestJsonTagCustomName(t *testing.T) {
	type T struct {
		FieldName string `json:"field_name"`
	}

	v := T{FieldName: "value"}
	got, err := Marshal(v)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	want, _ := json.Marshal(v)
	if string(got) != string(want) {
		t.Errorf("got %s, want %s", got, want)
	}
}

// --- 6. 字符串转义测试 ---

func TestStringEscapes(t *testing.T) {
	cases := []struct {
		input  string
		expect string
	}{
		{`"hello"`, "hello"},
		{`"hello\nworld"`, "hello\nworld"},
		{`"hello\tworld"`, "hello\tworld"},
		{`"hello\rworld"`, "hello\rworld"},
		{`"hello\bworld"`, "hello\bworld"},
		{`"hello\fworld"`, "hello\fworld"},
		{`"hello\\world"`, "hello\\world"},
		{`"hello\"world"`, "hello\"world"},
		{`"hello\/world"`, "hello/world"},
		{`"\u0041\u0042\u0043"`, "ABC"},
		{`"\u4e2d\u6587"`, "中文"},
		{`"\uD83D\uDE00"`, "😀"}, // surrogate pair
	}

	for _, c := range cases {
		var got string
		if err := Unmarshal([]byte(c.input), &got); err != nil {
			t.Errorf("Unmarshal(%s) error: %v", c.input, err)
			continue
		}
		if got != c.expect {
			t.Errorf("Unmarshal(%s) = %q, want %q", c.input, got, c.expect)
		}
	}
}

func TestMarshalStringEscapes(t *testing.T) {
	cases := []struct {
		input  string
		expect string
	}{
		{"hello", `"hello"`},
		{"hello\nworld", `"hello\nworld"`},
		{"hello\tworld", `"hello\tworld"`},
		{"hello\\world", `"hello\\world"`},
		{"hello\"world", `"hello\"world"`},
		{"中文", `"中文"`},
		{"\x00", `"\u0000"`},
		{"\x1f", `"\u001f"`},
	}

	for _, c := range cases {
		got, err := Marshal(c.input)
		if err != nil {
			t.Errorf("Marshal(%q) error: %v", c.input, err)
			continue
		}
		if string(got) != c.expect {
			t.Errorf("Marshal(%q) = %s, want %s", c.input, got, c.expect)
		}
	}
}

// --- 7. 数组/切片测试 ---

func TestSliceTypes(t *testing.T) {
	t.Run("[]int", func(t *testing.T) {
		input := `[1,2,3,4,5]`
		var got, want []int
		if err := Unmarshal([]byte(input), &got); err != nil {
			t.Fatalf("error: %v", err)
		}
		json.Unmarshal([]byte(input), &want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("[]float64", func(t *testing.T) {
		input := `[1.1,2.2,3.3]`
		var got, want []float64
		if err := Unmarshal([]byte(input), &got); err != nil {
			t.Fatalf("error: %v", err)
		}
		json.Unmarshal([]byte(input), &want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("[]string", func(t *testing.T) {
		input := `["a","b","c"]`
		var got, want []string
		if err := Unmarshal([]byte(input), &got); err != nil {
			t.Fatalf("error: %v", err)
		}
		json.Unmarshal([]byte(input), &want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("[][]int", func(t *testing.T) {
		input := `[[1,2],[3,4]]`
		var got, want [][]int
		if err := Unmarshal([]byte(input), &got); err != nil {
			t.Fatalf("error: %v", err)
		}
		json.Unmarshal([]byte(input), &want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("[]struct", func(t *testing.T) {
		type Item struct {
			Name  string `json:"name"`
			Price int    `json:"price"`
		}
		input := `[{"name":"a","price":1},{"name":"b","price":2}]`
		var got, want []Item
		if err := Unmarshal([]byte(input), &got); err != nil {
			t.Fatalf("error: %v", err)
		}
		json.Unmarshal([]byte(input), &want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("[]*struct", func(t *testing.T) {
		type Item struct {
			Name  string `json:"name"`
			Price int    `json:"price"`
		}
		input := `[{"name":"a","price":1},null,{"name":"b","price":2}]`
		var got, want []*Item
		if err := Unmarshal([]byte(input), &got); err != nil {
			t.Fatalf("error: %v", err)
		}
		json.Unmarshal([]byte(input), &want)
		if len(got) != len(want) {
			t.Fatalf("len got=%d, want=%d", len(got), len(want))
		}
		for i := range got {
			if (got[i] == nil) != (want[i] == nil) {
				t.Errorf("index %d: got nil=%v, want nil=%v", i, got[i] == nil, want[i] == nil)
			}
			if got[i] != nil && *got[i] != *want[i] {
				t.Errorf("index %d: got %+v, want %+v", i, *got[i], *want[i])
			}
		}
	})
}

func TestEmptySlice(t *testing.T) {
	t.Run("empty array", func(t *testing.T) {
		var got []int
		if err := Unmarshal([]byte("[]"), &got); err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %v, want empty", got)
		}
	})

	t.Run("null array", func(t *testing.T) {
		var got []int = []int{1, 2, 3}
		if err := Unmarshal([]byte("null"), &got); err != nil {
			t.Fatalf("error: %v", err)
		}
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
}

// --- 8. Map 类型测试 ---

func TestMapTypes(t *testing.T) {
	t.Run("map[string]string", func(t *testing.T) {
		input := `{"a":"1","b":"2"}`
		var got, want map[string]string
		if err := Unmarshal([]byte(input), &got); err != nil {
			t.Fatalf("error: %v", err)
		}
		json.Unmarshal([]byte(input), &want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("map[string]int", func(t *testing.T) {
		input := `{"a":1,"b":2}`
		var got, want map[string]int
		if err := Unmarshal([]byte(input), &got); err != nil {
			t.Fatalf("error: %v", err)
		}
		json.Unmarshal([]byte(input), &want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("map[string]interface{}", func(t *testing.T) {
		input := `{"a":1,"b":"hello","c":true,"d":[1,2]}`
		var got, want map[string]interface{}
		if err := Unmarshal([]byte(input), &got); err != nil {
			t.Fatalf("error: %v", err)
		}
		json.Unmarshal([]byte(input), &want)
		// Compare via re-marshal since interface{} comparison is tricky
		gotBytes, _ := json.Marshal(got)
		wantBytes, _ := json.Marshal(want)
		if string(gotBytes) != string(wantBytes) {
			t.Errorf("got %s, want %s", gotBytes, wantBytes)
		}
	})
}

// --- 9. interface{} 解码测试 ---

func TestDecodeToInterface(t *testing.T) {
	cases := []struct {
		input string
		check func(v interface{}) bool
	}{
		{"null", func(v interface{}) bool { return v == nil }},
		{"true", func(v interface{}) bool { b, _ := v.(bool); return b }},
		{"false", func(v interface{}) bool { b, _ := v.(bool); return !b }},
		{"42", func(v interface{}) bool { n, _ := v.(float64); return n == 42 }},
		{"3.14", func(v interface{}) bool { n, _ := v.(float64); return n == 3.14 }},
		{`"hello"`, func(v interface{}) bool { s, _ := v.(string); return s == "hello" }},
		{`[1,2,3]`, func(v interface{}) bool { a, _ := v.([]interface{}); return len(a) == 3 }},
		{`{"a":1}`, func(v interface{}) bool { m, _ := v.(map[string]interface{}); return m["a"] == float64(1) }},
	}

	for _, c := range cases {
		var got interface{}
		if err := Unmarshal([]byte(c.input), &got); err != nil {
			t.Errorf("Unmarshal(%s) error: %v", c.input, err)
			continue
		}
		if !c.check(got) {
			t.Errorf("Unmarshal(%s) = %v, check failed", c.input, got)
		}
	}
}

// --- 10. 深层嵌套测试 ---

func TestDeepNesting(t *testing.T) {
	// 20层嵌套
	depth := 20
	input := ""
	for i := 0; i < depth; i++ {
		input += `{"a":`
	}
	input += `1`
	for i := 0; i < depth; i++ {
		input += `}`
	}

	var got interface{}
	if err := Unmarshal([]byte(input), &got); err != nil {
		t.Fatalf("Unmarshal deep nesting error: %v", err)
	}

	// 验证深度
	v := got
	for i := 0; i < depth; i++ {
		m, ok := v.(map[string]interface{})
		if !ok {
			t.Fatalf("depth %d: expected map, got %T", i, v)
		}
		v = m["a"]
	}
	if v != float64(1) {
		t.Errorf("innermost value = %v, want 1", v)
	}
}

func TestDeepArrayNesting(t *testing.T) {
	depth := 20
	input := ""
	for i := 0; i < depth; i++ {
		input += `[`
	}
	input += `1`
	for i := 0; i < depth; i++ {
		input += `]`
	}

	var got interface{}
	if err := Unmarshal([]byte(input), &got); err != nil {
		t.Fatalf("Unmarshal deep array error: %v", err)
	}

	v := got
	for i := 0; i < depth; i++ {
		a, ok := v.([]interface{})
		if !ok {
			t.Fatalf("depth %d: expected array, got %T", i, v)
		}
		v = a[0]
	}
	if v != float64(1) {
		t.Errorf("innermost value = %v, want 1", v)
	}
}

// --- 11. 大整数精度测试 ---

func TestLargeNumberToFloat64(t *testing.T) {
	// JSON 数字超过 int64 范围，应作为 float64 处理
	input := `{"id": 9007199254740993}` // 2^53+1, 超过 float64 精确表示范围

	var got map[string]interface{}
	if err := Unmarshal([]byte(input), &got); err != nil {
		t.Fatalf("error: %v", err)
	}

	// 标准库也是 float64
	var want map[string]interface{}
	json.Unmarshal([]byte(input), &want)

	if got["id"] != want["id"] {
		t.Errorf("got %v (%T), want %v (%T)", got["id"], got["id"], want["id"], want["id"])
	}
}

// --- 12. 错误处理测试 ---

func TestInvalidJSON(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty", ``},
		{"invalid char", `{abc}`},
		{"unclosed string", `{"key":"value}`},
		{"unclosed object", `{"key":"value"`},
		{"unclosed array", `[1,2,3`},
		{"trailing comma", `{"a":1,}`},
		{"trailing comma array", `[1,2,]`},
		{"missing colon", `{"a" 1}`},
		{"missing value", `{"a":}`},
		{"double comma", `[1,,2]`},
		{"extra content", `{"a":1}extra`},
		{"invalid number", `1.2.3`},
		{"invalid keyword", `tru`},
		{"invalid unicode", `"\u00G"`},
		{"bare value in object", `{"a":,}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var v interface{}
			err := Unmarshal([]byte(c.input), &v)
			if err == nil {
				t.Errorf("expected error for input %q, got nil (result: %v)", c.input, v)
			}
		})
	}
}

// --- 13. Unicode 字符串测试 ---

func TestUnicodeStrings(t *testing.T) {
	cases := []string{
		"中文测试",
		"日本語テスト",
		"한국어 테스트",
		"Русский",
		"العربية",
		"emoji 🎉🚀💡",
		"mixed 中文 English 日本語",
	}

	for _, s := range cases {
		t.Run(s, func(t *testing.T) {
			// Marshal then Unmarshal round-trip
			data, err := Marshal(s)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}

			// Compare with encoding/json
			stdData, _ := json.Marshal(s)
			if string(data) != string(stdData) {
				t.Errorf("Marshal: got %s, want %s", data, stdData)
			}

			// Unmarshal back
			var got string
			if err := Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			if got != s {
				t.Errorf("round-trip: got %q, want %q", got, s)
			}
		})
	}
}

// --- 14. Marshal/Unmarshal 往返测试 ---

func TestRoundTrip(t *testing.T) {
	type Complex struct {
		Name      string                 `json:"name"`
		Age       int                    `json:"age"`
		Active    bool                   `json:"active"`
		Score     float64                `json:"score"`
		Tags      []string               `json:"tags"`
		Metadata  map[string]interface{} `json:"metadata"`
		Children  []Complex              `json:"children,omitempty"`
		Optional  *int                   `json:"optional,omitempty"`
	}

	v := Complex{
		Name:   "root",
		Age:    50,
		Active: true,
		Score:  99.5,
		Tags:   []string{"tag1", "tag2"},
		Metadata: map[string]interface{}{
			"key1": "value1",
			"key2": 42,
			"key3": true,
		},
	}
	optVal := 10
	v.Optional = &optVal

	// Marshal
	data, err := Marshal(v)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// Unmarshal
	var got Complex
	if err := Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// Compare key fields
	if got.Name != v.Name || got.Age != v.Age || got.Active != v.Active {
		t.Errorf("basic fields mismatch: got %+v", got)
	}
	if got.Score != v.Score {
		t.Errorf("score mismatch: got %v, want %v", got.Score, v.Score)
	}
	if !reflect.DeepEqual(got.Tags, v.Tags) {
		t.Errorf("tags mismatch: got %v, want %v", got.Tags, v.Tags)
	}
	if got.Optional == nil || *got.Optional != optVal {
		t.Errorf("optional mismatch: got %v, want %d", got.Optional, optVal)
	}
}

// --- 15. 特殊浮点数测试 ---

func TestSpecialFloat(t *testing.T) {
	t.Run("NaN", func(t *testing.T) {
		data, err := Marshal(math.NaN())
		if err != nil {
			t.Logf("Marshal NaN returned error (acceptable): %v", err)
			return
		}
		t.Logf("Marshal NaN = %s", data)
	})

	t.Run("Inf", func(t *testing.T) {
		data, err := Marshal(math.Inf(1))
		if err != nil {
			t.Logf("Marshal +Inf returned error (acceptable): %v", err)
			return
		}
		t.Logf("Marshal +Inf = %s", data)
	})
}

// --- 16. []byte 编解码测试 ---

func TestByteSlice(t *testing.T) {
	t.Run("encode []byte", func(t *testing.T) {
		b := []byte("hello")
		got, err := Marshal(b)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		want, _ := json.Marshal(b)
		// encoding/json encodes []byte as base64 string
		if string(got) != string(want) {
			t.Errorf("got %s, want %s", got, want)
		}
	})

	t.Run("encode nil []byte", func(t *testing.T) {
		var b []byte
		got, err := Marshal(b)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		want, _ := json.Marshal(b)
		if string(got) != string(want) {
			t.Errorf("got %s, want %s", got, want)
		}
	})
}

// --- 17. 固定长度数组测试 ---

func TestFixedArray(t *testing.T) {
	var got [3]int
	if err := Unmarshal([]byte(`[1,2,3]`), &got); err != nil {
		t.Fatalf("error: %v", err)
	}
	if got != [3]int{1, 2, 3} {
		t.Errorf("got %v, want [1 2 3]", got)
	}

	// 数组长度大于 JSON 数组
	var got2 [5]int
	if err := Unmarshal([]byte(`[1,2,3]`), &got2); err != nil {
		t.Fatalf("error: %v", err)
	}
	if got2 != [5]int{1, 2, 3, 0, 0} {
		t.Errorf("got %v, want [1 2 3 0 0]", got2)
	}
}

// --- 18. 流式解码测试 ---

func TestUnmarshalFromReader(t *testing.T) {
	input := `{"name":"test","value":42}`
	type T struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}
	var got T
	if err := UnmarshalFromReader(strings.NewReader(input), &got); err != nil {
		t.Fatalf("error: %v", err)
	}
	if got.Name != "test" || got.Value != 42 {
		t.Errorf("got %+v", got)
	}
}

// --- 19. 配置测试 ---

func TestSortMapKeys(t *testing.T) {
	m := map[string]int{"c": 3, "a": 1, "b": 2}

	// 默认不排序
	_, _ = Marshal(m)
	// 标准库默认排序
	stdData, _ := json.Marshal(m)

	// 排序后
	SetDefaultConfig(Config{SortMapKeys: true})
	data2, _ := Marshal(m)
	SetDefaultConfig(Config{SortMapKeys: false})

	// 排序后的输出应该和标准库一致
	if string(data2) != string(stdData) {
		t.Errorf("sorted: got %s, want %s", data2, stdData)
	}
}

// --- 20. 重复键测试 ---

func TestDuplicateKeys(t *testing.T) {
	type T struct {
		A int `json:"a"`
	}
	input := `{"a":1,"a":2}`

	var got T
	if err := Unmarshal([]byte(input), &got); err != nil {
		t.Fatalf("error: %v", err)
	}
	// encoding/json uses last value
	var want T
	json.Unmarshal([]byte(input), &want)
	if got.A != want.A {
		t.Errorf("got A=%d, want A=%d", got.A, want.A)
	}
}

// --- 21. 未知字段处理测试 ---

func TestUnknownFields(t *testing.T) {
	type T struct {
		Name string `json:"name"`
	}
	input := `{"name":"test","unknown":123,"nested":{"deep":true}}`

	var got T
	if err := Unmarshal([]byte(input), &got); err != nil {
		t.Fatalf("error: %v", err)
	}
	if got.Name != "test" {
		t.Errorf("got Name=%q, want %q", got.Name, "test")
	}
}

// --- 22. 大 JSON 测试 ---

func TestLargeJSON(t *testing.T) {
	// 生成一个较大的 JSON
	items := make([]map[string]interface{}, 100)
	for i := 0; i < 100; i++ {
		items[i] = map[string]interface{}{
			"id":          i,
			"name":        fmt.Sprintf("item_%d", i),
			"price":       float64(i) * 1.5,
			"available":   i%2 == 0,
			"tags":        []string{fmt.Sprintf("tag_%d", i), "common"},
			"description": fmt.Sprintf("This is item number %d with some description text", i),
		}
	}

	data, err := Marshal(items)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var got []map[string]interface{}
	if err := Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(got) != 100 {
		t.Errorf("got %d items, want 100", len(got))
	}

	// Check first item
	if got[0]["id"] != float64(0) {
		t.Errorf("first item id = %v, want 0", got[0]["id"])
	}
}

// --- 23. 前后空白字符测试 ---

func TestWhitespaceHandling(t *testing.T) {
	cases := []string{
		`  {"a":1}  `,
		"\n{\"a\":1}\n",
		"\t\t{\"a\":1}\t\t",
		`{"a" : 1}`,
		`{ "a" : 1 }`,
		`{"a":1 ,"b":2}`,
	}

	for _, input := range cases {
		var got map[string]int
		if err := Unmarshal([]byte(input), &got); err != nil {
			t.Errorf("Unmarshal(%q) error: %v", input, err)
		}
		if got["a"] != 1 {
			t.Errorf("Unmarshal(%q): a=%d, want 1", input, got["a"])
		}
	}
}

// --- 24. 数值到字符串的类型转换测试 ---

func TestNumberToString(t *testing.T) {
	// encoding/json 会报错
	input := `{"id":12345}`
	type T struct {
		ID string `json:"id"`
	}

	var got T
	err := Unmarshal([]byte(input), &got)

	// 检查行为：要么报错（和标准库一致），要么转换
	var want T
	stdErr := json.Unmarshal([]byte(input), &want)

	if (err == nil) != (stdErr == nil) {
		t.Errorf("error mismatch: sjson err=%v, std err=%v", err, stdErr)
	}
}
