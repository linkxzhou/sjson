package sjson

import (
	"math"
	"strings" // Need strings for complex JSON example
	"testing"
)

func TestLexer_NextToken(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []Token
	}{
		{
			name:  "基本标记",
			input: "{}[]:,",
			expected: []Token{
				{Type: LeftBraceToken, Value: []byte("{"), Pos: 0},
				{Type: RightBraceToken, Value: []byte("}"), Pos: 1},
				{Type: LeftBracketToken, Value: []byte("["), Pos: 2},
				{Type: RightBracketToken, Value: []byte("]"), Pos: 3},
				{Type: ColonToken, Value: []byte(":"), Pos: 4},
				{Type: CommaToken, Value: []byte(","), Pos: 5},
				{Type: EOFToken, Pos: 6},
			},
		},
		{
			name:  "数字",
			input: "123 -456 7.89 1e10 -2.5e-3",
			expected: []Token{
				{Type: IntegerToken, FloatValue: 123, Pos: 0},
				{Type: IntegerToken, FloatValue: -456, Pos: 4},
				{Type: FloatToken, FloatValue: 7.89, Pos: 9},
				{Type: FloatToken, FloatValue: 1e10, Pos: 14},
				{Type: FloatToken, FloatValue: -2.5e-3, Pos: 19},
				{Type: EOFToken, Pos: 26},
			},
		},
		{
			name:  "关键字",
			input: "true false null",
			expected: []Token{
				{Type: TrueToken, Value: []byte("true"), Pos: 0},
				{Type: FalseToken, Value: []byte("false"), Pos: 5},
				{Type: NullToken, Value: []byte("null"), Pos: 11},
				{Type: EOFToken, Pos: 15},
			},
		},
		{
			name:  "基本字符串",
			input: `"hello" "world"`,
			expected: []Token{
				{Type: StringToken, Value: []byte("hello"), Pos: 0},
				{Type: StringToken, Value: []byte("world"), Pos: 8},
				{Type: EOFToken, Pos: 15},
			},
		},
		{
			name:  "Unicode转义",
			input: `"\u0041\u4F60\u597D"`,
			expected: []Token{
				{Type: StringToken, Value: []byte("A你好"), Pos: 0},
				{Type: EOFToken, Pos: 20},
			},
		},
		{
			name:  "简单JSON对象",
			input: `{"name":"张三", "age":30}`,
			expected: []Token{
				{Type: LeftBraceToken, Value: []byte("{"), Pos: 0},
				{Type: StringToken, Value: []byte("name"), Pos: 1},
				{Type: ColonToken, Value: []byte(":"), Pos: 7},
				{Type: StringToken, Value: []byte("张三"), Pos: 8},     // "张三" is 6 bytes in UTF-8
				{Type: CommaToken, Value: []byte(","), Pos: 16},      // Position after "张三"
				{Type: StringToken, Value: []byte("age"), Pos: 18},   // Position after ", "
				{Type: ColonToken, Value: []byte(":"), Pos: 23},      // Position after "age"
				{Type: IntegerToken, FloatValue: 30, Pos: 24},        // Position after ":"
				{Type: RightBraceToken, Value: []byte("}"), Pos: 26}, // Position after "30"
				{Type: EOFToken, Pos: 27},                            // Position after "}"
			},
		},
		{
			name:  "边界数字测试",
			input: "0 -0 123456789 -987654321",
			expected: []Token{
				{Type: IntegerToken, FloatValue: 0, Pos: 0},
				{Type: IntegerToken, FloatValue: 0, Pos: 2},
				{Type: IntegerToken, FloatValue: 123456789, Pos: 5},
				{Type: IntegerToken, FloatValue: -987654321, Pos: 15},
				{Type: EOFToken, Pos: 25},
			},
		},
		{
			name:  "小数点数字测试",
			input: "0.0 123.456 -789.012 0.123456789",
			expected: []Token{
				{Type: FloatToken, FloatValue: 0.0, Pos: 0},
				{Type: FloatToken, FloatValue: 123.456, Pos: 4},
				{Type: FloatToken, FloatValue: -789.012, Pos: 12},
				{Type: FloatToken, FloatValue: 0.123456789, Pos: 21},
				{Type: EOFToken, Pos: 32},
			},
		},
		{
			name:  "科学计数法测试",
			input: "1e0 1E0 1e+0 1E+0 1e-0 1E-0 123e45 -456E-78 1.23e+10 -4.56E-20",
			expected: []Token{
				{Type: FloatToken, FloatValue: 1e0, Pos: 0},
				{Type: FloatToken, FloatValue: 1e0, Pos: 4},
				{Type: FloatToken, FloatValue: 1e+0, Pos: 8},
				{Type: FloatToken, FloatValue: 1e+0, Pos: 13},
				{Type: FloatToken, FloatValue: 1e-0, Pos: 18},
				{Type: FloatToken, FloatValue: 1e-0, Pos: 23},
				{Type: FloatToken, FloatValue: 123e45, Pos: 28},
				{Type: FloatToken, FloatValue: -456e-78, Pos: 35},
				{Type: FloatToken, FloatValue: 1.23e+10, Pos: 44},
				{Type: FloatToken, FloatValue: -4.56e-20, Pos: 53},
				{Type: EOFToken, Pos: 62},
			},
		},
		{
			name:  "大数字测试",
			input: "9007199254740991 -9007199254740991 1234567890123456",
			expected: []Token{
				{Type: IntegerToken, FloatValue: 9007199254740991, Pos: 0},
				{Type: IntegerToken, FloatValue: -9007199254740991, Pos: 17},
				{Type: IntegerToken, FloatValue: 1234567890123456, Pos: 35},
				{Type: EOFToken, Pos: 51},
			},
		},
		{
			name:  "JSON数组中的数字",
			input: `[0, 1, -2, 3.14, -2.71, 1e10, -1.5e-3]`,
			expected: []Token{
				{Type: LeftBracketToken, Value: []byte("["), Pos: 0},
				{Type: IntegerToken, FloatValue: 0, Pos: 1},
				{Type: CommaToken, Value: []byte(","), Pos: 2},
				{Type: IntegerToken, FloatValue: 1, Pos: 4},
				{Type: CommaToken, Value: []byte(","), Pos: 5},
				{Type: IntegerToken, FloatValue: -2, Pos: 7},
				{Type: CommaToken, Value: []byte(","), Pos: 9},
				{Type: FloatToken, FloatValue: 3.14, Pos: 11},
				{Type: CommaToken, Value: []byte(","), Pos: 15},
				{Type: FloatToken, FloatValue: -2.71, Pos: 17},
				{Type: CommaToken, Value: []byte(","), Pos: 22},
				{Type: FloatToken, FloatValue: 1e10, Pos: 24},
				{Type: CommaToken, Value: []byte(","), Pos: 28},
				{Type: FloatToken, FloatValue: -1.5e-3, Pos: 30},
				{Type: RightBracketToken, Value: []byte("]"), Pos: 37},
				{Type: EOFToken, Pos: 38},
			},
		},
		{
			name:  "复杂JSON对象中的数字",
			input: `{"id":12345,"price":99.99,"discount":0.15,"quantity":-5,"weight":1.5e-2}`,
			expected: []Token{
				{Type: LeftBraceToken, Value: []byte("{"), Pos: 0},
				{Type: StringToken, Value: []byte("id"), Pos: 1},
				{Type: ColonToken, Value: []byte(":"), Pos: 5},
				{Type: IntegerToken, FloatValue: 12345, Pos: 6},
				{Type: CommaToken, Value: []byte(","), Pos: 11},
				{Type: StringToken, Value: []byte("price"), Pos: 12},
				{Type: ColonToken, Value: []byte(":"), Pos: 19},
				{Type: FloatToken, FloatValue: 99.99, Pos: 20},
				{Type: CommaToken, Value: []byte(","), Pos: 25},
				{Type: StringToken, Value: []byte("discount"), Pos: 26},
				{Type: ColonToken, Value: []byte(":"), Pos: 36},
				{Type: FloatToken, FloatValue: 0.15, Pos: 37},
				{Type: CommaToken, Value: []byte(","), Pos: 41},
				{Type: StringToken, Value: []byte("quantity"), Pos: 42},
				{Type: ColonToken, Value: []byte(":"), Pos: 52},
				{Type: IntegerToken, FloatValue: -5, Pos: 53},
				{Type: CommaToken, Value: []byte(","), Pos: 55},
				{Type: StringToken, Value: []byte("weight"), Pos: 56},
				{Type: ColonToken, Value: []byte(":"), Pos: 64},
				{Type: FloatToken, FloatValue: 1.5e-2, Pos: 65},
				{Type: RightBraceToken, Value: []byte("}"), Pos: 71},
				{Type: EOFToken, Pos: 72},
			},
		},
		{
			name:  "嵌套JSON中的数字",
			input: `{"stats":{"min":0,"max":100,"avg":75.5},"scores":[85,92,78]}`,
			expected: []Token{
				{Type: LeftBraceToken, Value: []byte("{"), Pos: 0},
				{Type: StringToken, Value: []byte("stats"), Pos: 1},
				{Type: ColonToken, Value: []byte(":"), Pos: 8},
				{Type: LeftBraceToken, Value: []byte("{"), Pos: 9},
				{Type: StringToken, Value: []byte("min"), Pos: 10},
				{Type: ColonToken, Value: []byte(":"), Pos: 15},
				{Type: IntegerToken, FloatValue: 0, Pos: 16},
				{Type: CommaToken, Value: []byte(","), Pos: 17},
				{Type: StringToken, Value: []byte("max"), Pos: 18},
				{Type: ColonToken, Value: []byte(":"), Pos: 23},
				{Type: IntegerToken, FloatValue: 100, Pos: 24},
				{Type: CommaToken, Value: []byte(","), Pos: 27},
				{Type: StringToken, Value: []byte("avg"), Pos: 28},
				{Type: ColonToken, Value: []byte(":"), Pos: 33},
				{Type: FloatToken, FloatValue: 75.5, Pos: 34},
				{Type: RightBraceToken, Value: []byte("}"), Pos: 38},
				{Type: CommaToken, Value: []byte(","), Pos: 39},
				{Type: StringToken, Value: []byte("scores"), Pos: 40},
				{Type: ColonToken, Value: []byte(":"), Pos: 48},
				{Type: LeftBracketToken, Value: []byte("["), Pos: 49},
				{Type: IntegerToken, FloatValue: 85, Pos: 50},
				{Type: CommaToken, Value: []byte(","), Pos: 52},
				{Type: IntegerToken, FloatValue: 92, Pos: 53},
				{Type: CommaToken, Value: []byte(","), Pos: 55},
				{Type: IntegerToken, FloatValue: 78, Pos: 56},
				{Type: RightBracketToken, Value: []byte("]"), Pos: 58},
				{Type: RightBraceToken, Value: []byte("}"), Pos: 59},
				{Type: EOFToken, Pos: 60},
			},
		},
		{
			name:  "混合数据类型JSON",
			input: `{"active":true,"count":42,"ratio":0.618,"items":[1,2,3],"meta":null}`,
			expected: []Token{
				{Type: LeftBraceToken, Value: []byte("{"), Pos: 0},
				{Type: StringToken, Value: []byte("active"), Pos: 1},
				{Type: ColonToken, Value: []byte(":"), Pos: 9},
				{Type: TrueToken, Value: []byte("true"), Pos: 10},
				{Type: CommaToken, Value: []byte(","), Pos: 14},
				{Type: StringToken, Value: []byte("count"), Pos: 15},
				{Type: ColonToken, Value: []byte(":"), Pos: 22},
				{Type: IntegerToken, FloatValue: 42, Pos: 23},
				{Type: CommaToken, Value: []byte(","), Pos: 25},
				{Type: StringToken, Value: []byte("ratio"), Pos: 26},
				{Type: ColonToken, Value: []byte(":"), Pos: 33},
				{Type: FloatToken, FloatValue: 0.618, Pos: 34},
				{Type: CommaToken, Value: []byte(","), Pos: 39},
				{Type: StringToken, Value: []byte("items"), Pos: 40},
				{Type: ColonToken, Value: []byte(":"), Pos: 47},
				{Type: LeftBracketToken, Value: []byte("["), Pos: 48},
				{Type: IntegerToken, FloatValue: 1, Pos: 49},
				{Type: CommaToken, Value: []byte(","), Pos: 50},
				{Type: IntegerToken, FloatValue: 2, Pos: 51},
				{Type: CommaToken, Value: []byte(","), Pos: 52},
				{Type: IntegerToken, FloatValue: 3, Pos: 53},
				{Type: RightBracketToken, Value: []byte("]"), Pos: 54},
				{Type: CommaToken, Value: []byte(","), Pos: 55},
				{Type: StringToken, Value: []byte("meta"), Pos: 56},
				{Type: ColonToken, Value: []byte(":"), Pos: 62},
				{Type: NullToken, Value: []byte("null"), Pos: 63},
				{Type: RightBraceToken, Value: []byte("}"), Pos: 67},
				{Type: EOFToken, Pos: 68},
			},
		},
		{
			name:  "极小数字测试",
			input: "1e-100 -1e-100 1.23456789e-50",
			expected: []Token{
				{Type: FloatToken, FloatValue: 1e-100, Pos: 0},
				{Type: FloatToken, FloatValue: -1e-100, Pos: 7},
				{Type: FloatToken, FloatValue: 1.23456789e-50, Pos: 15},
				{Type: EOFToken, Pos: 29},
			},
		},
	}

	for _, tt := range tests {
		testCase := tt // 避免闭包问题
		t.Run(testCase.name, func(t *testing.T) {
			lexer := NewLexer([]byte(testCase.input))
			for i, expected := range testCase.expected {
				got := lexer.NextToken()
				if got.Type != expected.Type {
					t.Errorf("标记 #%d 类型错误: 期望 %v, 得到 %v", i, expected.Type, got.Type)
				}

				if got.Type == IntegerToken || got.Type == FloatToken {
					if math.Abs(got.FloatValue-expected.FloatValue) > 1e-6 {
						t.Errorf("标记 #%d 值错误: 期望 %f, 得到 %f", i, expected.FloatValue, got.FloatValue)
					}
				} else {
					if string(got.Value) != string(expected.Value) {
						t.Errorf("标记 #%d 值错误: 期望 %q, 得到 %q", i, expected.Value, got.Value)
					}
				}

				if got.Pos != expected.Pos {
					t.Errorf("标记 #%d 位置错误: 期望 %d, 得到 %d", i, expected.Pos, got.Pos)
				}
			}
		})
	}
}

// Benchmark for simple JSON input
func BenchmarkLexerSimple(b *testing.B) {
	input := []byte(`{
		"name": "张三",
		"age": 30,
		"city": "北京",
		"active": true,
		"scores": [100, 95, 88]
	}`)
	for i := 0; i < b.N; i++ {
		lexer := NewLexer(input)
		for {
			token := lexer.NextToken()
			if token.Type == EOFToken {
				break
			}
		}
	}
}

// Benchmark for a slightly more complex JSON input
func BenchmarkLexerComplex(b *testing.B) {
	// Generate a larger JSON string for more realistic benchmarking
	var sb strings.Builder
	sb.WriteString(`{
		"widget": {
			"debug": "on",
			"window": {
				"title": "Sample Konfabulator Widget",
				"name": "main_window",
				"width": 500,
				"height": 500
			},
			"image": {
				"src": "Images/Sun.png",
				"name": "sun1",
				"hOffset": 250,
				"vOffset": 250,
				"alignment": "center"
			},
			"text": {
				"data": "Click Here",
				"size": 36,
				"style": "bold",
				"name": "text1",
				"hOffset": 250,
				"vOffset": 100,
				"alignment": "center",
				"onMouseUp": "sun1.opacity = (sun1.opacity / 100) * 90;"
			}
		}
	}`)
	input := []byte(sb.String())

	b.ResetTimer() // Reset timer after setup
	for i := 0; i < b.N; i++ {
		lexer := NewLexer(input)
		for {
			token := lexer.NextToken()
			if token.Type == EOFToken {
				break
			}
		}
	}
}

// Benchmark focusing on string parsing with escapes
func BenchmarkLexerStringEscapes(b *testing.B) {
	input := []byte(`{"escapes": "Hello\nWorld\tTab\"Quote\\Backslash\u0041\u4F60\u597D"}`)
	for i := 0; i < b.N; i++ {
		lexer := NewLexer(input)
		for {
			token := lexer.NextToken()
			if token.Type == EOFToken {
				break
			}
		}
	}
}

// BenchmarkLexerAllTokens 测试包含所有token类型的复杂JSON解析性能
func BenchmarkLexerAllTokens(b *testing.B) {
	// 构建包含所有类型Token的JSON
	jsonStr := []byte(`{
		"nullValue": null,
		"boolValues": {
			"trueValue": true,
			"falseValue": false
		},
		"numbers": [
			0,
			42,
			-73,
			3.14159,
			-2.71828,
			1.23e+6,
			-4.56e-3
		],
		"strings": [
			"",
			"Hello, World!",
			"特殊字符: \\, \", \/, \b, \f, \n, \r, \t",
			"Unicode: \u0041\u4F60\u597D"
		],
		"nestedObjects": {
			"level1": {
				"level2": {
					"level3": {
						"deep": "nested value"
					},
					"array": [1, 2, [3, 4, [5]]]
				}
			}
		},
		"mixedArray": [
			null,
			true,
			false,
			123,
			-45.67,
			"string",
			[1, 2, 3],
			{"key": "value"}
		],
		"emptyStructures": {
			"emptyObject": {},
			"emptyArray": []
		}
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lexer := NewLexer(jsonStr)
		for {
			token := lexer.NextToken()
			if token.Type == EOFToken {
				break
			}
		}
	}
}

// BenchmarkLexerBatchProcessing 测试批量处理性能
func BenchmarkLexerBatchProcessing(b *testing.B) {
	// 使用相同的复杂JSON
	jsonStr := []byte(`{
		"nullValue": null,
		"boolValues": {
			"trueValue": true,
			"falseValue": false
		},
		"numbers": [
			0,
			42,
			-73,
			3.14159,
			-2.71828,
			1.23e+6,
			-4.56e-3
		],
		"strings": [
			"",
			"Hello, World!",
			"特殊字符: \\, \", \/, \b, \f, \n, \r, \t",
			"Unicode: \u0041\u4F60\u597D"
		],
		"nestedObjects": {
			"level1": {
				"level2": {
					"level3": {
						"deep": "nested value"
					},
					"array": [1, 2, [3, 4, [5]]]
				}
			}
		},
		"mixedArray": [
			null,
			true,
			false,
			123,
			-45.67,
			"string",
			[1, 2, 3],
			{"key": "value"}
		],
		"emptyStructures": {
			"emptyObject": {},
			"emptyArray": []
		}
	}`)

	b.Run("单个标记处理", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			lexer := NewLexer(jsonStr)
			for {
				token := lexer.NextToken()
				if token.Type == EOFToken {
					break
				}
			}
		}
	})
}

// 新增专门的数字解析测试函数
func TestLexer_NumberParsing(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  float64
		tokenType TokenType
	}{
		// 整数测试
		{"零", "0", 0, IntegerToken},
		{"正整数", "123", 123, IntegerToken},
		{"负整数", "-456", -456, IntegerToken},
		{"大整数", "9007199254740991", 9007199254740991, IntegerToken},
		{"负大整数", "-9007199254740991", -9007199254740991, IntegerToken},

		// 浮点数测试
		{"零点零", "0.0", 0.0, FloatToken},
		{"正小数", "123.456", 123.456, FloatToken},
		{"负小数", "-789.012", -789.012, FloatToken},
		{"小于1的小数", "0.123456789", 0.123456789, FloatToken},
		{"负小于1的小数", "-0.987654321", -0.987654321, FloatToken},

		// 科学计数法测试
		{"正指数", "1e10", 1e10, FloatToken},
		{"负指数", "1e-10", 1e-10, FloatToken},
		{"大写E", "1E10", 1e10, FloatToken},
		{"带加号指数", "1e+10", 1e+10, FloatToken},
		{"小数科学计数法", "1.23e10", 1.23e10, FloatToken},
		{"负数科学计数法", "-4.56e-20", -4.56e-20, FloatToken},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := NewLexer([]byte(tt.input))
			token := lexer.NextToken()

			if token.Type != tt.tokenType {
				t.Errorf("标记类型错误: 期望 %v, 得到 %v", tt.tokenType, token.Type)
			}

			if token.Type == IntegerToken || token.Type == FloatToken {
				if math.Abs(token.FloatValue-tt.expected) > 1e-6 {
					t.Errorf("值错误: 期望 %f, 得到 %f", tt.expected, token.FloatValue)
				}
			} else {
				if string(token.Value) != string(tt.input) {
					t.Errorf("值错误: 期望 %q, 得到 %q", tt.input, token.Value)
				}
			}
		})
	}
}

// 新增JSON数字解析基准测试
func BenchmarkLexerNumbers(b *testing.B) {
	tests := []struct {
		name  string
		input []byte
	}{
		{
			"整数数组",
			[]byte(`[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20]`),
		},
		{
			"浮点数数组",
			[]byte(`[1.1,2.2,3.3,4.4,5.5,6.6,7.7,8.8,9.9,10.1,11.2,12.3,13.4,14.5]`),
		},
		{
			"科学计数法数组",
			[]byte(`[1e1,2e2,3e3,4e-1,5e-2,6e+3,7E4,8E-5,9E+6,1.5e10]`),
		},
		{
			"混合数字JSON",
			[]byte(`{"integers":[1,2,3],"floats":[1.1,2.2,3.3],"scientific":[1e10,2e-5]}`),
		},
		{
			"大数字数组",
			[]byte(`[9223372036854775807,-9223372036854775808,1.7976931348623157e+308]`),
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				lexer := NewLexer(tt.input)
				for {
					token := lexer.NextToken()
					if token.Type == EOFToken {
						break
					}
				}
			}
		})
	}
}

// 新增数字精度测试
func TestLexer_NumberPrecision(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected float64
	}{
		{"高精度小数", "3.141592653589793", 3.141592653589793},
		{"高精度负数", "-2.718281828459045", -2.718281828459045},
		{"高精度科学计数法", "6.62607015e-34", 6.62607015e-34},
		{"高精度大数", "1.23456789012345e+20", 1.23456789012345e+20},
		{"高精度小数", "9.87654321098765e-15", 9.87654321098765e-15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := NewLexer([]byte(tt.input))
			token := lexer.NextToken()

			if token.Type != FloatToken {
				t.Errorf("期望浮点数标记，得到 %v", token.Type)
			}

			// 使用相对误差检查，因为浮点数精度限制
			diff := token.FloatValue - tt.expected
			if diff < 0 {
				diff = -diff
			}
			relativeError := diff / tt.expected
			if relativeError > 1e-15 {
				t.Errorf("精度错误: 期望 %.17g, 得到 %.17g, 相对误差 %.2e",
					tt.expected, token.FloatValue, relativeError)
			}
		})
	}
}
