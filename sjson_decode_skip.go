package sjson

import (
	"fmt"
)

// skipValue 跳过一个JSON值
// 使用字节级快速跳过，避免完整的Token解析
func (d *Decoder) skipValue() error {
	switch d.token.Type {
	case NullToken, TrueToken, FalseToken, IntegerToken, FloatToken, StringToken:
		// 简单值，直接跳过
		d.nextToken()
		return nil

	case LeftBraceToken:
		// 跳过对象 - 使用字节级快速跳过
		return d.skipObjectFast()

	case LeftBracketToken:
		// 跳过数组 - 使用字节级快速跳过
		return d.skipArrayFast()

	default:
		return fmt.Errorf("无法跳过未知的JSON标记: %v", d.token)
	}
}

// skipObjectFast 字节级快速跳过对象
// 直接在原始字节上扫描，不做完整的Token解析
func (d *Decoder) skipObjectFast() error {
	input := d.lexer.input
	pos := d.lexer.pos
	inputLen := d.lexer.inputLen

	depth := 1 // 已经读取了 { token

	for pos < inputLen && depth > 0 {
		c := input[pos]

		switch c {
		case '{':
			depth++
			pos++
		case '}':
			depth--
			pos++
		case '[':
			depth++
			pos++
		case ']':
			depth--
			pos++
		case '"':
			// 快速跳过字符串
			pos++
			for pos < inputLen {
				c := input[pos]
				if c == '\\' && pos+1 < inputLen {
					// 跳过转义字符
					pos += 2
					continue
				}
				if c == '"' {
					pos++
					break
				}
				pos++
			}
		case ' ', '\t', '\n', '\r', ',', ':':
			// 跳过空白和分隔符
			pos++
		default:
			// 其他字符（数字、字母等），跳过直到遇到结构字符
			pos++
			for pos < inputLen {
				c := input[pos]
				if c == '{' || c == '}' || c == '[' || c == ']' ||
					c == '"' || c == ',' || c == ':' ||
					c == ' ' || c == '\t' || c == '\n' || c == '\r' {
					break
				}
				pos++
			}
		}
	}

	if depth != 0 {
		return fmt.Errorf("对象未正确闭合")
	}

	// 更新Lexer位置并读取下一个token
	d.lexer.pos = pos
	d.lexer.start = pos
	d.nextToken()
	return nil
}

// skipArrayFast 字节级快速跳过数组
func (d *Decoder) skipArrayFast() error {
	input := d.lexer.input
	pos := d.lexer.pos
	inputLen := d.lexer.inputLen

	depth := 1 // 已经读取了 [ token

	for pos < inputLen && depth > 0 {
		c := input[pos]

		switch c {
		case '[':
			depth++
			pos++
		case ']':
			depth--
			pos++
		case '{':
			depth++
			pos++
		case '}':
			depth--
			pos++
		case '"':
			// 快速跳过字符串
			pos++
			for pos < inputLen {
				c := input[pos]
				if c == '\\' && pos+1 < inputLen {
					pos += 2
					continue
				}
				if c == '"' {
					pos++
					break
				}
				pos++
			}
		case ' ', '\t', '\n', '\r', ',', ':':
			pos++
		default:
			pos++
			for pos < inputLen {
				c := input[pos]
				if c == '{' || c == '}' || c == '[' || c == ']' ||
					c == '"' || c == ',' || c == ':' ||
					c == ' ' || c == '\t' || c == '\n' || c == '\r' {
					break
				}
				pos++
			}
		}
	}

	if depth != 0 {
		return fmt.Errorf("数组未正确闭合")
	}

	d.lexer.pos = pos
	d.lexer.start = pos
	d.nextToken()
	return nil
}

// 以下是旧的实现，保留作为备用

// skipObject 跳过对象 - 使用Token解析版本
func (d *Decoder) skipObject() error {
	// 跳过左大括号
	d.nextToken()

	// 空对象快速处理
	if d.token.Type == RightBraceToken {
		d.nextToken()
		return nil
	}

	// 使用深度计数来跳过嵌套结构
	depth := 1

	for depth > 0 {
		switch d.token.Type {
		case EOFToken:
			return fmt.Errorf("对象未正确闭合")

		case LeftBraceToken:
			depth++
			d.nextToken()

		case RightBraceToken:
			depth--
			d.nextToken()
			if depth == 0 {
				return nil
			}

		case LeftBracketToken:
			// 进入数组，需要追踪数组深度
			d.nextToken()
			arrayDepth := 1
			for arrayDepth > 0 && d.token.Type != EOFToken {
				switch d.token.Type {
				case LeftBracketToken:
					arrayDepth++
				case RightBracketToken:
					arrayDepth--
				case LeftBraceToken:
					// 数组中的对象，增加对象深度
					depth++
					d.nextToken()
					continue
				}
				d.nextToken()
			}

		default:
			d.nextToken()
		}
	}

	return nil
}

// skipArray 跳过数组 - 使用Token解析版本
func (d *Decoder) skipArray() error {
	// 跳过左方括号
	d.nextToken()

	// 空数组快速处理
	if d.token.Type == RightBracketToken {
		d.nextToken()
		return nil
	}

	// 使用深度计数
	depth := 1

	for depth > 0 {
		switch d.token.Type {
		case EOFToken:
			return fmt.Errorf("数组未正确闭合")

		case LeftBracketToken:
			depth++
			d.nextToken()

		case RightBracketToken:
			depth--
			d.nextToken()
			if depth == 0 {
				return nil
			}

		case LeftBraceToken:
			// 进入对象，需要追踪对象深度
			d.nextToken()
			objDepth := 1
			for objDepth > 0 && d.token.Type != EOFToken {
				switch d.token.Type {
				case LeftBraceToken:
					objDepth++
				case RightBraceToken:
					objDepth--
				case LeftBracketToken:
					// 对象中的数组，增加数组深度
					depth++
					d.nextToken()
					continue
				}
				d.nextToken()
			}

		default:
			d.nextToken()
		}
	}

	return nil
}
