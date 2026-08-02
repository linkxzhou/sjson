package sjson

import (
	"bytes"
	"io"
	"strconv"
	"sync"
	"unicode/utf8"
	"unsafe"
)

// TokenType 表示词法标记的类型
type TokenType int

const (
	InvalidToken      TokenType = iota // 无效的标记
	EOFToken                           // 文件结束标记
	IntegerToken                       // 整数标记，例如：123, -456
	FloatToken                         // 浮点数标记，例如：45.67, 1.23e-4
	StringToken                        // 字符串标记，例如："hello"
	NullToken                          // null值标记
	TrueToken                          // true布尔值标记
	FalseToken                         // false布尔值标记
	CommaToken                         // 逗号标记 ','
	ColonToken                         // 冒号标记 ':'
	LeftBraceToken                     // 左大括号标记 '{'
	RightBraceToken                    // 右大括号标记 '}'
	LeftBracketToken                   // 左方括号标记 '['
	RightBracketToken                  // 右方括号标记 ']'
)

// 预先定义的字节切片常量，避免重复创建
var (
	leftBraceByte    = []byte("{")
	rightBraceByte   = []byte("}")
	leftBracketByte  = []byte("[")
	rightBracketByte = []byte("]")
	commaByte        = []byte(",")
	colonByte        = []byte(":")
	nullByte         = []byte("null")
	trueByte         = []byte("true")
	falseByte        = []byte("false")
)

// Token 表示一个词法标记
type Token struct {
	Type      TokenType
	FloatValue float64
	IntValue   int64
	IsInteger  bool
	Value      []byte // 字符串值 / 数字原始字节（合并原 RawNumber，省 24B slice header）
	Pos        int
}

// Lexer 用于将JSON文本转换为标记流
type Lexer struct {
	input    []byte
	inputLen int
	pos      int
	start    int
	width    int
}

// 用于复用 bytes.Buffer
var bufferPool = sync.Pool{
	New: func() interface{} {
		return &bytes.Buffer{}
	},
}

// NewLexer 创建一个新的词法分析器
func NewLexer(input []byte) *Lexer {
	return &Lexer{input: input, inputLen: len(input)}
}

// NewLexerFromReader 从io.Reader创建一个新的词法分析器
func NewLexerFromReader(r io.Reader) (*Lexer, error) {
	// 直接读取所有数据到字节切片，避免中间的Buffer拷贝
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	// 直接创建词法分析器，无需额外拷贝
	return NewLexer(data), nil
}

// Reset 重置词法分析器状态，用于复用
func (l *Lexer) Reset(input []byte) {
	l.input = input
	l.inputLen = len(input)
	l.pos = 0
	l.start = 0
	l.width = 0
}

// next 返回下一个字符并前进
func (l *Lexer) next() rune {
	if l.pos >= len(l.input) {
		l.width = 0
		return -1
	}

	r, w := utf8.DecodeRune(l.input[l.pos:])
	l.width = w
	l.pos += w
	return r
}

// ignore 忽略当前标记中已扫描的文本
func (l *Lexer) ignore() {
	l.start = l.pos
}

// NextToken 返回下一个标记
func (l *Lexer) NextToken() Token {
	l.start = l.pos
	inputLen := l.inputLen
	// 快速跳过空白字符（8字节批量处理）
	for l.pos+8 <= inputLen {
		// 一次读取8个字节
		chunk := *(*uint64)(unsafe.Pointer(&l.input[l.pos]))

		// 检查是否全部都是空白字符
		// 空格(0x20), 换行(0x0A), 制表符(0x09), 回车(0x0D)
		if !isAllWhitespace8(chunk) {
			// 不全是空白字符，需要逐字节处理
			break
		}

		l.pos += 8
	}

	// 处理剩余字节
	for l.pos < inputLen {
		c := l.input[l.pos]
		if c == ' ' || c == '\n' || c == '\t' || c == '\r' {
			l.pos++
		} else {
			break
		}
	}
	l.start = l.pos // 更新标记起始位置

	// 检查EOF
	if l.pos >= inputLen {
		return Token{Type: EOFToken, Value: nil, Pos: l.start}
	}

	// 基于当前字节快速确定标记类型
	c := l.input[l.pos]
	l.pos++ // 提前移动位置，大多数单字符标记只需要读一个字节

	// 处理单字符标记（最常见的情况）
	switch c {
	case '{':
		return Token{Type: LeftBraceToken, Value: leftBraceByte, Pos: l.start}
	case '}':
		return Token{Type: RightBraceToken, Value: rightBraceByte, Pos: l.start}
	case '[':
		return Token{Type: LeftBracketToken, Value: leftBracketByte, Pos: l.start}
	case ']':
		return Token{Type: RightBracketToken, Value: rightBracketByte, Pos: l.start}
	case ',':
		return Token{Type: CommaToken, Value: commaByte, Pos: l.start}
	case ':':
		return Token{Type: ColonToken, Value: colonByte, Pos: l.start}
	case '"': // 字符串
		l.pos-- // 回退，因为lexString需要读取引号
		return l.lexString()
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9': // 数字
		l.pos-- // 回退
		return l.lexNumber()
	}

	// 处理关键字：直接检查开头并内联比较
	if c == 'n' && inputLen-l.start >= 4 &&
		bytes.Equal(l.input[l.start:l.start+4], nullByte) {
		l.pos = l.start + 4
		return Token{Type: NullToken, Value: nullByte, Pos: l.start}
	} else if c == 't' && inputLen-l.start >= 4 &&
		bytes.Equal(l.input[l.start:l.start+4], trueByte) {
		l.pos = l.start + 4
		return Token{Type: TrueToken, Value: trueByte, Pos: l.start}
	} else if c == 'f' && inputLen-l.start >= 5 &&
		bytes.Equal(l.input[l.start:l.start+5], falseByte) {
		l.pos = l.start + 5
		return Token{Type: FalseToken, Value: falseByte, Pos: l.start}
	} else {
		// pass
	}

	// 无效标记
	return Token{Type: InvalidToken, Value: []byte{c}, Pos: l.start}
}

// lexString 解析字符串标记
func (l *Lexer) lexString() Token {
	startPos := l.start // 保存标记开始位置

	// 跳过起始引号（这里已经确认是 '"'）
	l.pos++
	l.start = l.pos // 忽略起始引号，start 指向内容开始

	// 快速路径：无转义字符的情况
	// 直接在原始输入上操作，零拷贝
	start := l.pos
	inputLen := l.inputLen
	
	// 快速路径：一次处理8个字节
	for l.pos+8 <= inputLen {
		// 一次读取8个字节
		chunk := *(*uint64)(unsafe.Pointer(&l.input[l.pos]))

		// 检查是否包含引号 (0x22) 或反斜杠 (0x5C)
		// 使用位运算快速检测特殊字符
		hasQuote := hasBytes8(chunk, 0x2222222222222222)     // 8个引号
		hasBackslash := hasBytes8(chunk, 0x5C5C5C5C5C5C5C5C) // 8个反斜杠
		hasControl := hasControlChars8(chunk)                // 控制字符 < 0x20

		if hasQuote || hasBackslash || hasControl {
			// 有特殊字符，逐字节处理这8个字节
			break
		}

		l.pos += 8
	}

	// 处理剩余的字节（不足8个或遇到特殊字符）
	for l.pos < inputLen {
		c := l.input[l.pos]
		if c == '"' {
			// 直接返回原始输入的切片，零拷贝
			value := l.input[start:l.pos]
			l.pos++ // 跳过结束引号
			return Token{Type: StringToken, Value: value, Pos: startPos}
		}
		if c == '\\' {
			// 遇到转义字符，需要进入慢路径
			break
		}
		// 检查无效字符（控制字符）
		if c < 0x20 {
			return Token{Type: InvalidToken, Value: []byte("字符串中包含无效控制字符"), Pos: l.pos}
		}
		l.pos++
	}

	// 慢路径：处理带转义字符的情况
	return l.lexStringEscape(startPos, start)
}

// lexStringEscape 处理带转义字符的字符串（慢路径）
func (l *Lexer) lexStringEscape(startPos, contentStart int) Token {
	// 只有在遇到转义字符时才使用buffer
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufferPool.Put(buf)

	// 预分配内存减少重新分配
	if buf.Cap() < 32 {
		buf.Grow(32)
	}

	// 将已经扫描的无转义部分写入buffer
	if l.pos > contentStart {
		buf.Write(l.input[contentStart:l.pos])
	}

	inputLen := l.inputLen
	
	for l.pos < inputLen {
		c := l.input[l.pos]
		
		if c == '\\' {
			l.pos++
			if l.pos >= inputLen {
				return Token{Type: InvalidToken, Value: []byte("未闭合的字符串 (EOF after escape)"), Pos: startPos}
			}
			
			// 处理转义序列
			esc := l.input[l.pos]
			l.pos++
			
			switch esc {
			case '"', '\\', '/':
				buf.WriteByte(esc)
			case 'b':
				buf.WriteByte('\b')
			case 'f':
				buf.WriteByte('\f')
			case 'n':
				buf.WriteByte('\n')
			case 'r':
				buf.WriteByte('\r')
			case 't':
				buf.WriteByte('\t')
			case 'u':
				// Unicode转义处理
				if l.pos+4 > inputLen {
					return Token{Type: InvalidToken, Value: []byte("无效的 Unicode 转义序列 (过短)"), Pos: l.pos - 1}
				}

				hex := l.input[l.pos : l.pos+4]
				code, _, err := parseIntFromBytes(hex, 16, 32)
				if err != nil {
					return Token{Type: InvalidToken, Value: []byte("无效的 Unicode 转义序列"), Pos: l.pos - 1}
				}
				l.pos += 4

				// 处理 UTF-16 代理对
				if code >= 0xD800 && code <= 0xDBFF {
					// 高代理项，检查是否有低代理项
					if l.pos+6 <= inputLen &&
						l.input[l.pos] == '\\' && l.input[l.pos+1] == 'u' {
						hex2 := l.input[l.pos+2 : l.pos+6]
						code2, _, err2 := parseIntFromBytes(hex2, 16, 32)
						if err2 == nil && code2 >= 0xDC00 && code2 <= 0xDFFF {
							// 合并代理对
							r := 0x10000 + ((code - 0xD800) << 10) + (code2 - 0xDC00)
							l.pos += 6
							buf.WriteRune(rune(r))
							continue
						}
					}
					// 孤立高代理项：替换为 U+FFFD
					buf.WriteRune('\uFFFD')
					continue
				}

				if code >= 0xDC00 && code <= 0xDFFF {
					// 孤立低代理项：替换为 U+FFFD
					buf.WriteRune('\uFFFD')
					continue
				}

				buf.WriteRune(rune(code))
			default:
				return Token{Type: InvalidToken, Value: []byte("无效的转义字符"), Pos: l.pos - 2}
			}
		} else if c == '"' {
			l.pos++ // 跳过结束引号
			// 创建结果副本
			result := append([]byte(nil), buf.Bytes()...)
			return Token{Type: StringToken, Value: result, Pos: startPos}
		} else if c < 0x20 {
			return Token{Type: InvalidToken, Value: []byte("字符串中包含无效控制字符"), Pos: l.pos}
		} else {
			// 普通字符，写入buffer
			buf.WriteByte(c)
			l.pos++
		}
	}

	return Token{Type: InvalidToken, Value: []byte("未闭合的字符串"), Pos: startPos}
}

// lexNumber 解析数字标记
//
// 注：早期尝试过对 IntegerToken 不计算 FloatValue 以省一次 float64 转换，
// 但 interface{} 装箱路径依赖 FloatValue（与 encoding/json 保持 float64 一致）；
// 改为保留 FloatValue，去重只在 decodeNumber 内部做。
func (l *Lexer) lexNumber() Token {
	startPos := l.start
	inputLen := l.inputLen
	start := l.start

	// 扫描数字字节，确定是否为浮点数
	isFloat := false
	pos := start

	// 处理符号
	if pos < inputLen && (l.input[pos] == '-' || l.input[pos] == '+') {
		pos++
	}

	// 整数部分
	hasDigits := false
	if pos < inputLen && l.input[pos] == '0' {
		// 前导零：仅允许 "0" 或 "0." 或 "0e/E"
		pos++
		hasDigits = true
	} else {
		for pos < inputLen && l.input[pos] >= '0' && l.input[pos] <= '9' {
			pos++
			hasDigits = true
		}
	}

	// 小数部分
	if pos < inputLen && l.input[pos] == '.' {
		isFloat = true
		pos++
		// 小数部分必须至少有一位数字
		fracStart := pos
		for pos < inputLen && l.input[pos] >= '0' && l.input[pos] <= '9' {
			pos++
		}
		if pos == fracStart {
			return Token{Type: InvalidToken, Value: []byte("无效的浮点数格式: 缺少小数部分"), Pos: startPos}
		}
	}

	// 指数部分
	if pos < inputLen && (l.input[pos] == 'e' || l.input[pos] == 'E') {
		isFloat = true
		pos++
		if pos < inputLen && (l.input[pos] == '+' || l.input[pos] == '-') {
			pos++
		}
		expStart := pos
		for pos < inputLen && l.input[pos] >= '0' && l.input[pos] <= '9' {
			pos++
		}
		if pos == expStart {
			return Token{Type: InvalidToken, Value: []byte("无效的指数格式: 缺少指数数字"), Pos: startPos}
		}
	}

	if !hasDigits {
		return Token{Type: InvalidToken, Value: []byte("无效的数字格式"), Pos: startPos}
	}

	// 保存原始字节
	raw := l.input[start:pos]
	l.pos = pos

	if isFloat {
		// 使用 strconv.ParseFloat 保证精度
		n, err := strconv.ParseFloat(bytesToString(raw), 64)
		if err != nil {
			return Token{Type: InvalidToken, Value: []byte("浮点数解析失败: " + err.Error()), Pos: startPos}
		}
		return Token{Type: FloatToken, FloatValue: n, Value: raw, Pos: startPos}
	}

	// 整数：先尝试快速解析（零分配），失败再回退 strconv
	// 同步计算 FloatValue 以保持 interface{} 装箱语义与 encoding/json 一致
	// 注意：IsInteger=true 仅在 IntValue 有效时设置；溢出时置 false，
	// 解码侧据此回退到 FloatValue / strconv 链（与原行为一致）。
	if n, ok := parseInt64Fast(raw); ok {
		return Token{Type: IntegerToken, IntValue: n, FloatValue: float64(n), Value: raw, IsInteger: true, Pos: startPos}
	}
	// 快速解析失败（溢出或无效），用 float64
	fn, ferr := strconv.ParseFloat(bytesToString(raw), 64)
	if ferr != nil {
		return Token{Type: InvalidToken, Value: []byte("数字解析失败"), Pos: startPos}
	}
	return Token{Type: IntegerToken, FloatValue: fn, Value: raw, IsInteger: false, Pos: startPos}
}
