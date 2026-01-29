package sjson

import (
	"bytes"
	"io"
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
	Type       TokenType
	FloatValue float64
	Value      []byte
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
func (l *Lexer) lexNumber() Token {
	startPos := l.start
	inputLen := l.inputLen
	n, i, isFloat, err := parseFloatFromBytes(l.input[l.start:inputLen], 64)
	if err != nil && err != errInvalidDigitError {
		return Token{Type: InvalidToken, Value: []byte(err.Error()), Pos: startPos}
	}

	// 跳过解析的字符数
	l.pos += i

	// 根据是否为浮点数返回不同的token类型
	if isFloat {
		return Token{Type: FloatToken, FloatValue: n, Pos: startPos}
	}

	return Token{Type: IntegerToken, FloatValue: n, Pos: startPos}
}
