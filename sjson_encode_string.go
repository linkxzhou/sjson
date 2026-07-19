package sjson

import (
	"encoding/base64"
	"reflect"
	"unicode/utf8"
	"unsafe"
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
//
//go:inline
func (e stringEncoder) appendToBytes(stream *encoderStream, src reflect.Value) error {
	return encodeStringDirect(stream, src.String())
}

// stringNeedsEscapeSWAR 使用 SWAR（SIMD within a register）技术一次检查 8 字节
// 检测是否有需要转义的字符（< 0x20, ", \）
//
//go:inline
func stringNeedsEscapeSWAR(s string) bool {
	n := len(s)
	if n == 0 {
		return false
	}

	// 8 字节批量检测
	for n >= 8 {
		chunk := stringChunk64(s)
		// 检测引号 (0x22)
		if hasBytes8(chunk, 0x2222222222222222) {
			return true
		}
		// 检测反斜杠 (0x5C)
		if hasBytes8(chunk, 0x5C5C5C5C5C5C5C5C) {
			return true
		}
		// 检测控制字符 (< 0x20) 或非 ASCII (>= 0x80)
		// hasControlChars8 用 (chunk - 0x20) & 0x80 检测：
		//   - 字节 < 0x20: 减 0x20 后借位，高位置 1
		//   - 字节 >= 0x80: 减 0x20 后高位仍为 1
		// 两者都需要逐字节处理，因此统一返回 true
		if hasControlChars8(chunk) {
			// 需要区分：真正的控制字符 vs 非 ASCII
			// 非 ASCII 字符不需要转义（如果是合法 UTF-8），但需要逐字节检查
			// 控制字符需要转义
			// 这里保守返回 true，进入逐字节路径
			return true
		}
		s = s[8:]
		n -= 8
	}

	// 尾部逐字节处理剩余 0-7 字节
	for i := 0; i < n; i++ {
		c := s[i]
		if c >= 0x80 {
			// 非 ASCII 字符需要逐字节处理
			return true
		}
		if !safeSet[c] {
			return true
		}
	}

	return false
}

// stringChunk64 读取字符串前 8 字节为 uint64
//
//go:inline
func stringChunk64(s string) uint64 {
	return *(*uint64)(unsafe.Pointer(unsafe.StringData(s)))
}

// encodeStringDirect 直接编码字符串，避免反射
//
//go:inline
func encodeStringDirect(stream *encoderStream, s string) error {
	if s == "" {
		stream.buffer = append(stream.buffer, emptyString...)
		return nil
	}

	// OPT-3: SWAR 快速路径——一次检测 8 字节判断是否需要转义
	needsEscape := stringNeedsEscapeSWAR(s)

	if !needsEscape {
		// 无需转义，直接添加
		stream.buffer = append(stream.buffer, '"')
		stream.buffer = append(stream.buffer, s...)
		stream.buffer = append(stream.buffer, '"')
		return nil
	}

	// 需要转义，预分配足够的空间
	needed := len(s) + 2
	if cap(stream.buffer)-len(stream.buffer) < needed*2 {
		newCap := cap(stream.buffer) * 2
		if newCap < len(stream.buffer)+needed*2 {
			newCap = len(stream.buffer) + needed*2
		}
		newBuf := make([]byte, len(stream.buffer), newCap)
		copy(newBuf, stream.buffer)
		stream.buffer = newBuf
	}

	stream.buffer = append(stream.buffer, '"')

	// 需要转义，单次循环处理
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

// []byte 专用编码器（base64 编码，与 encoding/json 一致）
type byteSliceEncoder struct{}

func (e byteSliceEncoder) appendToBytes(stream *encoderStream, src reflect.Value) error {
	if src.IsNil() {
		stream.buffer = append(stream.buffer, nullString...)
		return nil
	}

	b := src.Bytes()
	if len(b) == 0 {
		stream.buffer = append(stream.buffer, emptyString...)
		return nil
	}

	stream.buffer = append(stream.buffer, '"')
	enc := base64.StdEncoding.EncodeToString(b)
	stream.buffer = append(stream.buffer, enc...)
	stream.buffer = append(stream.buffer, '"')
	return nil
}
