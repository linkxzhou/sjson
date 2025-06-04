package sjson

import (
	"reflect"
	"unicode/utf8"
)

// 预分配的 Unicode 控制字符转义表
var unicodeHex = [32]string{
	"\\u0000", "\\u0001", "\\u0002", "\\u0003", "\\u0004", "\\u0005", "\\u0006", "\\u0007",
	"\\u0008", "\\u0009", "\\u000a", "\\u000b", "\\u000c", "\\u000d", "\\u000e", "\\u000f",
	"\\u0010", "\\u0011", "\\u0012", "\\u0013", "\\u0014", "\\u0015", "\\u0016", "\\u0017",
	"\\u0018", "\\u0019", "\\u001a", "\\u001b", "\\u001c", "\\u001d", "\\u001e", "\\u001f",
}

// ASCII 安全字符集，不需要转义的字符标记为 true
var safeSet = [utf8.RuneSelf]bool{
	' ': true, '!': true, '#': true, '$': true, '%': true, '&': true, '\'': true,
	'(': true, ')': true, '*': true, '+': true, ',': true, '-': true, '.': true,
	'/': true, '0': true, '1': true, '2': true, '3': true, '4': true, '5': true,
	'6': true, '7': true, '8': true, '9': true, ':': true, ';': true, '<': true,
	'=': true, '>': true, '?': true, '@': true, 'A': true, 'B': true, 'C': true,
	'D': true, 'E': true, 'F': true, 'G': true, 'H': true, 'I': true, 'J': true,
	'K': true, 'L': true, 'M': true, 'N': true, 'O': true, 'P': true, 'Q': true,
	'R': true, 'S': true, 'T': true, 'U': true, 'V': true, 'W': true, 'X': true,
	'Y': true, 'Z': true, '[': true, ']': true, '^': true, '_': true, '`': true,
	'a': true, 'b': true, 'c': true, 'd': true, 'e': true, 'f': true, 'g': true,
	'h': true, 'i': true, 'j': true, 'k': true, 'l': true, 'm': true, 'n': true,
	'o': true, 'p': true, 'q': true, 'r': true, 's': true, 't': true, 'u': true,
	'v': true, 'w': true, 'x': true, 'y': true, 'z': true, '{': true, '|': true,
	'}': true, '~': true,
}

// 重置安全字符集中需要转义的特殊字符
func init() {
	safeSet['"'] = false
	safeSet['\\'] = false
	safeSet['\t'] = false
	safeSet['\n'] = false
	safeSet['\r'] = false
	safeSet['\b'] = false
	safeSet['\f'] = false
	// 控制字符也不安全
	for i := 0; i < 32; i++ {
		safeSet[i] = false
	}
}

//go:inline
func escapeStringToBytes(buf []byte, c byte) []byte {
	switch c {
	case '"':
		buf = append(buf, '\\', '"')
	case '\\':
		buf = append(buf, '\\', '\\')
	case '\b':
		buf = append(buf, '\\', 'b')
	case '\f':
		buf = append(buf, '\\', 'f')
	case '\n':
		buf = append(buf, '\\', 'n')
	case '\r':
		buf = append(buf, '\\', 'r')
	case '\t':
		buf = append(buf, '\\', 't')
	default:
		// 小于32的控制字符需要转义为\uXXXX
		if c < 32 {
			buf = append(buf, unicodeHex[c]...)
		} else {
			buf = append(buf, c)
		}
	}
	return buf
}

type stringEncoder struct{}

// 为stringEncoder添加appendToBytes方法
func (e stringEncoder) appendToBytes(stream *encoderStream, src reflect.Value) error {
	s := src.String()

	if s == "" {
		stream.buffer = append(stream.buffer, emptyString...)
		return nil
	}

	stream.buffer = append(stream.buffer, '"')

	// 单次循环，边检查边处理
	start := 0
	for i := 0; i < len(s); {
		if c := s[i]; c < utf8.RuneSelf {
			if !safeSet[c] {
				// 需要转义的字符
				if start < i {
					stream.buffer = append(stream.buffer, s[start:i]...)
				}
				stream.buffer = escapeStringToBytes(stream.buffer, c)
				i++
				start = i
			} else {
				// 安全字符，继续
				i++
			}
		} else {
			// 处理非ASCII字符（UTF-8）
			_, size := utf8.DecodeRuneInString(s[i:])
			i += size
		}
	}

	// 添加剩余部分
	if start < len(s) {
		stream.buffer = append(stream.buffer, s[start:]...)
	}

	stream.buffer = append(stream.buffer, '"')
	return nil
}

// []byte 专用编码器
type byteSliceEncoder struct{}

// 为byteSliceEncoder添加appendToBytes方法
func (e byteSliceEncoder) appendToBytes(stream *encoderStream, src reflect.Value) error {
	if src.IsNil() {
		stream.buffer = append(stream.buffer, nullString...)
		return nil
	}

	// 编码字节切片为JSON字符串
	b := src.Bytes()
	if len(b) == 0 {
		stream.buffer = append(stream.buffer, emptyString...)
		return nil
	}

	stream.buffer = append(stream.buffer, '"')

	for i := 0; i < len(b); {
		if c := b[i]; c < utf8.RuneSelf {
			if safeSet[c] {
				stream.buffer = append(stream.buffer, c)
			} else {
				stream.buffer = escapeStringToBytes(stream.buffer, c)
			}
			i++
		} else {
			// 处理非ASCII字符（UTF-8）
			_, size := utf8.DecodeRune(b[i:])
			stream.buffer = append(stream.buffer, b[i:i+size]...)
			i += size
		}
	}

	stream.buffer = append(stream.buffer, '"')
	return nil
}
