package sjson

import (
	"errors"
	"math"
	"strconv"
	"unsafe"
)

var digits [][]byte
var singleDigits = [10]byte{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9'}
var errInvalidDigitError = errors.New("无效的数字字符")

func init() {
	// 预计算0-9999的字节表示
	digits = make([][]byte, 10000)
	for i := 0; i < 10000; i++ {
		digits[i] = strconv.AppendInt(nil, int64(i), 10)
	}
}

// 直接从字节切片解析整数，避免string转换
// 返回值增加 consumed：实际消费的字节数
func parseIntFromBytes(b []byte, base int, bitSize int) (int64, int, error) {
	lenB := len(b)
	if lenB == 0 {
		return 0, 0, errors.New("空字节切片")
	}

	// 处理符号
	var negative bool
	var i int
	if b[0] == '+' {
		i = 1
	} else if b[0] == '-' {
		negative = true
		i = 1
	}

	// 检查是否有数字
	if i >= lenB {
		return 0, i, errors.New("无效的数字格式")
	}

	// 计算值
	var n int64
	var err error

	// 针对十进制数字的批量处理优化
	if base == 10 {
		// 4字节批量处理优化（仅适用于十进制）
		for i+4 <= lenB {
			// 一次读取4个字节
			chunk := *(*uint32)(unsafe.Pointer(&b[i]))

			// 检查4个字节是否都是数字字符
			if !isAllDigits4(chunk) {
				// 不全是数字，回退到逐字节处理
				break
			}

			// 批量转换4个数字字符
			d1 := int64(b[i] - '0')
			d2 := int64(b[i+1] - '0')
			d3 := int64(b[i+2] - '0')
			d4 := int64(b[i+3] - '0')

			// 检查溢出（在乘法之前检查）
			if n > (math.MaxInt64-d4-d3*10-d2*100-d1*1000)/10000 {
				// 溢出处理
				if negative {
					return math.MinInt64, i, nil
				}
				return math.MaxInt64, i, nil
			}

			// 一次性计算4位数字的值
			n = n*10000 + d1*1000 + d2*100 + d3*10 + d4
			i += 4
		}

		// 处理剩余的字节（逐字节处理）
		for ; i < lenB; i++ {
			d := b[i]

			if d < '0' || d > '9' {
				err = errInvalidDigitError
				break
			}

			v := int64(d - '0')

			// 检查溢出
			if n > (math.MaxInt64-v)/10 {
				if negative {
					return math.MinInt64, i, nil
				}
				return math.MaxInt64, i, nil
			}

			n = n*10 + v
		}
	} else {
		// 非十进制数字的通用处理（保持原有逻辑）
		for ; i < lenB; i++ {
			d := b[i]

			var v byte

			switch {
			case '0' <= d && d <= '9':
				v = d - '0'
			case 'a' <= d && d <= 'z':
				v = d - 'a' + 10
			case 'A' <= d && d <= 'Z':
				v = d - 'A' + 10
			default:
				err = errInvalidDigitError
				break
			}

			if int(v) >= base {
				err = errors.New("数字超出进制范围")
				break
			}

			// 检查溢出
			if n > math.MaxInt64/int64(base) {
				// 溢出
				if negative {
					return math.MinInt64, i, nil
				}
				return math.MaxInt64, i, nil
			}

			n *= int64(base)
			n += int64(v)
		}
	}

	if negative {
		n = -n
	}

	// 根据bitSize检查范围
	switch bitSize {
	case 8:
		if n < math.MinInt8 || n > math.MaxInt8 {
			return 0, i, errors.New("数值超出int8范围")
		}
	case 16:
		if n < math.MinInt16 || n > math.MaxInt16 {
			return 0, i, errors.New("数值超出int16范围")
		}
	case 32:
		if n < math.MinInt32 || n > math.MaxInt32 {
			return 0, i, errors.New("数值超出int32范围")
		}
	}

	return n, i, err
}

// 直接从字节切片解析浮点数，避免string转换
// 返回值增加 consumed：实际消费的字节数
func parseFloatFromBytes(b []byte, bitSize int) (float64, int, bool, error) {
	lenB := len(b)
	isFloat := false
	if lenB == 0 {
		return 0, 0, isFloat, errors.New("空字节切片")
	}

	// 处理符号
	var negative bool
	var i int
	if b[0] == '+' {
		i = 1
	} else if b[0] == '-' {
		negative = true
		i = 1
	}

	// 解析整数部分
	var n float64
	var err error
	var hasDecimal bool

	for ; i < lenB; i++ {
		if b[i] == '.' {
			isFloat = true
			i++
			break
		}
		if b[i] == 'e' || b[i] == 'E' {
			break
		}
		if '0' <= b[i] && b[i] <= '9' {
			n = n*10 + float64(b[i]-'0')
		} else {
			err = errInvalidDigitError
			break
		}
	}

	// 解析小数部分
	if i < lenB && i > 0 && b[i-1] == '.' {
		isFloat = true
		decimal := 0.1
		for ; i < lenB; i++ {
			if b[i] == 'e' || b[i] == 'E' {
				hasDecimal = true
				break
			}
			if '0' <= b[i] && b[i] <= '9' {
				hasDecimal = true
				n += decimal * float64(b[i]-'0')
				decimal *= 0.1
			} else {
				err = errInvalidDigitError
				break
			}
		}
	}

	if b[i-1] == '.' && !hasDecimal {
		return 0, i, isFloat, errors.New("无效的浮点数格式")
	}

	// 处理指数部分
	if i < lenB && (b[i] == 'e' || b[i] == 'E') {
		isFloat = true
		i++
		if i >= lenB {
			return 0, i, isFloat, errors.New("无效的指数格式")
		}

		expSign := 1
		if b[i] == '+' {
			i++
		} else if b[i] == '-' {
			expSign = -1
			i++
		}

		if i >= lenB || b[i] < '0' || b[i] > '9' {
			return 0, i, isFloat, errors.New("无效的指数格式")
		}

		var exp int
		for ; i < lenB; i++ {
			if '0' <= b[i] && b[i] <= '9' {
				exp = exp*10 + int(b[i]-'0')
			} else {
				err = errInvalidDigitError
				break
			}
		}

		// 应用指数
		if expSign > 0 {
			for j := 0; j < exp; j++ {
				n *= 10
			}
		} else {
			for j := 0; j < exp; j++ {
				n /= 10
			}
		}
	}

	if negative {
		n = -n
		if i < 2 {
			return 0, i, isFloat, errors.New("无效的负数格式")
		}
	}

	// 根据bitSize检查范围
	if bitSize == 32 {
		// 直接转换为float32再转回float64，不做额外的范围检查
		// 如果值超出范围，Go 会自动处理为 Inf
		isFloat = true
		return float64(float32(n)), 0, isFloat, nil
	}

	return n, i, isFloat, err
}

// stringToBytes 将 string 转换为 []byte，零拷贝（不安全）
//
//go:inline
func stringToBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// bytesToString 将 []byte 转换为 string，零拷贝（不安全）
//
//go:inline
func bytesToString(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}

//go:inline
//go:nosplit
//go:nocheckptr
func appendUint(dst []byte, u uint64, base int) []byte {
	// 超快速路径：单个数字
	if u < 10 {
		return append(dst, singleDigits[u])
	}

	// 快速路径：小于10000直接查表
	if u < 10000 {
		return append(dst, digits[u]...)
	}

	// 中等数字优化：使用分组处理减少除法运算
	if u < 100000000 { // 小于1亿，使用优化算法
		return appendUintOptimized(dst, u)
	}

	// 大数处理：直接使用标准库
	return strconv.AppendUint(dst, u, base)
}

//go:inline
//go:nosplit
//go:nocheckptr
func appendInt(dst []byte, i int64, base int) []byte {
	if i < 0 {
		dst = append(dst, '-')
		return appendUint(dst, uint64(-i), base)
	}
	return appendUint(dst, uint64(i), base)
}

// 优化的中等数字处理函数，参考 Jsoniter 和高性能 itoa 实现
//
//go:inline
//go:nosplit
func appendUintOptimized(dst []byte, u uint64) []byte {
	// 预计算数字位数，避免重复计算
	var digitCount int
	temp := u
	for {
		digitCount++
		temp /= 10
		if temp == 0 {
			break
		}
	}

	// 预分配空间
	start := len(dst)
	dst = append(dst, make([]byte, digitCount)...)

	// 从右到左填充数字，使用查表优化
	pos := start + digitCount - 1
	for u >= 100 {
		// 每次处理两位数字，减少除法运算
		q := u / 100
		r := u % 100
		u = q

		// 使用预计算的两位数字表
		if r < 10 {
			dst[pos] = singleDigits[r]
			pos--
		} else {
			twoDigits := digits[r]
			dst[pos] = twoDigits[1]
			dst[pos-1] = twoDigits[0]
			pos -= 2
		}
	}

	// 处理剩余的1-2位数字
	if u >= 10 {
		twoDigits := digits[u]
		dst[pos] = twoDigits[1]
		dst[pos-1] = twoDigits[0]
	} else {
		dst[pos] = singleDigits[u]
	}

	return dst
}

// 检查8字节中是否包含指定字节
//
//go:nosplit
func hasBytes8(chunk uint64, pattern uint64) bool {
	// 使用SWAR (SIMD Within A Register) 技术
	// 每个字节与目标字节异或，如果相等则为0
	diff := chunk ^ pattern
	// 检查是否有任何字节为0
	// 使用位运算技巧：(x - 0x0101010101010101) & ^x & 0x8080808080808080
	return ((diff - 0x0101010101010101) & ^diff & 0x8080808080808080) != 0
}

// 检查8字节中是否包含控制字符 (< 0x20)
//
//go:nosplit
func hasControlChars8(chunk uint64) bool {
	// 检查每个字节是否小于0x20
	// 使用饱和减法技巧
	return ((chunk - 0x2020202020202020) & 0x8080808080808080) != 0
}

// 使用位运算的更高效版本
//
//go:nosplit
func isAllWhitespace8(chunk uint64) bool {
	// 使用查找表方式，将空白字符映射为位掩码
	// 空格(0x20)=32, 制表符(0x09)=9, 换行(0x0A)=10, 回车(0x0D)=13

	// 创建空白字符的位掩码 (位置9,10,13,32对应的位为1)
	const whitespaceMask = (1 << 9) | (1 << 10) | (1 << 13) | (1 << 32)

	// 检查每个字节
	for i := 0; i < 8; i++ {
		b := byte(chunk >> (i * 8))
		if b >= 64 || (whitespaceMask&(1<<b)) == 0 {
			return false
		}
	}
	return true
}

// 使用位运算的更高效版本
//
//go:nosplit
func isAllDigits4(chunk uint32) bool {
	// 数字字符范围是 0x30-0x39
	// 使用位运算技巧：先减去0x30，然后检查是否都小于10

	// 减去 0x30303030 (4个0x30)
	adjusted := chunk - 0x30303030

	// 检查每个字节是否小于10 (0x0A)
	// 如果任何字节 >= 10，则对应位会被设置
	mask := adjusted & 0xF0F0F0F0 // 检查高4位

	// 还需要检查是否有字节小于0x30 (下溢)
	underflow := (chunk ^ 0x30303030) & 0x80808080

	return mask == 0 && underflow == 0
}
