package sjson

import (
	"fmt"
)

// 跳过一个JSON值
func (d *Decoder) skipValue() error {
	switch d.token.Type {
	case NullToken, TrueToken, FalseToken, IntegerToken, FloatToken, StringToken:
		// 简单值，直接跳过
		d.nextToken()
		return nil

	case LeftBraceToken:
		// 跳过对象
		return d.skipObject()

	case LeftBracketToken:
		// 跳过数组
		return d.skipArray()

	default:
		return fmt.Errorf("无法跳过未知的JSON标记: %v", d.token)
	}
}

// 跳过对象 - 优化版本，使用深度计数而非递归
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

// 跳过数组 - 优化版本
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
