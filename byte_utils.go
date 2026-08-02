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

// head8 将字节切片的前 8 字节按小端序加载为 uint64，不足 8 字节时高位补零。
// 与长度组成 (len, head) 二元组可做字段名快速等值比较（jsoniter/sonic 同款技巧）：
// len<=8 时一次 uint64 相等比较即可精确判定，>8 时才需追加 bytes.Equal 校验。
//
//go:inline
func head8(b []byte) uint64 {
	n := len(b)
	if n >= 8 {
		return *(*uint64)(unsafe.Pointer(unsafe.SliceData(b)))
	}
	var u uint64
	for i := 0; i < n; i++ {
		u |= uint64(b[i]) << (8 * i)
	}
	return u
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
// 无借位污染版本：正确拒绝所有非 ASCII (>= 0x80) 字节。
// 判据：存在字节 b < 0x20 等价于 ((b - 0x20) & ~b & 0x80) != 0
//  - b < 0x20: 减 0x20 借位，~b 高位 1，AND 后高位 1
//  - b >= 0x20: 减 0x20 不借位或借用正常位，~b 高位 0，AND 后该位置 0
//  - b >= 0x80: 减 0x20 后高位 1（>0x80），但 ~b 高位 0，AND 后该位置 0
//
//go:nosplit
func hasControlChars8(chunk uint64) bool {
	return ((chunk - 0x2020202020202020) & ^chunk & 0x8080808080808080) != 0
}

// zeroByteMask 检测 uint64 中每个字节是否为 0，返回的 uint64 中零字节对应位置高比特为 1。
//
//go:inline
func zeroByteMask(x uint64) uint64 {
	const lo = 0x0101010101010101
	const hi = 0x8080808080808080
	return (x - lo) & ^x & hi
}

// isAllWhitespace8 判断 8 字节是否全部为 JSON 空白（空格 0x20 / \t 0x09 / \n 0x0A / \r 0x0D）。
// 用 SWAR 位运算一次性判定，无逐字节循环、无分支。
//
// 原理：对每个目标值 t，chunk XOR broadcast(t) 在 b==t 的字节处产生 0。
// 用 zeroByteMask 检测零字节位置，4 个目标的检测结果 OR 起来，
// 若全 8 字节都有至少一个命中，则掩码 == 0x8080808080808080。
//
//go:inline
func isAllWhitespace8(chunk uint64) bool {
	const hi = 0x8080808080808080
	hit := zeroByteMask(chunk^0x2020202020202020) |
		zeroByteMask(chunk^0x0909090909090909) |
		zeroByteMask(chunk^0x0A0A0A0A0A0A0A0A) |
		zeroByteMask(chunk^0x0D0D0D0D0D0D0D0D)
	return hit == hi
}

// 使用位运算的更高效版本
//
//go:nosplit
func isAllDigits4(chunk uint32) bool {
	// 数字字符范围是 '0'(0x30) - '9'(0x39)，每个字节都需满足：
	//   1) 高 4 位 = 0x3   （即 chunk & 0xF0F0F0F0 == 0x30303030）
	//   2) 低 4 位 <= 0x9   （即 (chunk + 0x06060606) & 0xF0F0F0F0 == 0x30303030）
	// 无借位污染，正确拒绝 ':' (0x3A) ';' '<' '=' '>' '?' 等临近字符。
	//
	// 验证：
	//   '9' = 0x39 → 0x30 ✓ / 0x3F+0x06=0x45, 0x45&0xF0=0x40 ≠ 0x30 ✗ 正确拒绝是不可能的数字
	//   实际：
	//   '9' (0x39): high nibble = 0x3 ✓; '+0x6' = 0x3F, &0xF0 = 0x30 ✓ 通过
	//   ':' (0x3A): high nibble = 0x3 ✓; '+0x6' = 0x40, &0xF0 = 0x40 ✗ 正确拒绝
	//   '/' (0x2F): high nibble = 0x2 ✗ 直接拒绝
	return (chunk & 0xF0F0F0F0) == 0x30303030 &&
		(chunk+0x06060606)&0xF0F0F0F0 == 0x30303030
}

// parseInt64Fast parses a base-10 int64 directly from byte slice.
// Returns (value, true) on success, (0, false) on overflow or invalid input.
// Uses SWAR 4-byte batch processing for speed, avoids string allocation.
//
//go:inline
func parseInt64Fast(b []byte) (int64, bool) {
	lenB := len(b)
	if lenB == 0 {
		return 0, false
	}

	var negative bool
	i := 0
	if b[0] == '-' {
		negative = true
		i = 1
	} else if b[0] == '+' {
		i = 1
	}

	if i >= lenB {
		return 0, false
	}

	var n uint64
	// SWAR: batch 4 digits at a time
	for i+4 <= lenB {
		chunk := *(*uint32)(unsafe.Pointer(&b[i]))
		if !isAllDigits4(chunk) {
			break
		}
		d1 := uint64(b[i] - '0')
		d2 := uint64(b[i+1] - '0')
		d3 := uint64(b[i+2] - '0')
		d4 := uint64(b[i+3] - '0')
		if n > (math.MaxUint64-d1*1000-d2*100-d3*10-d4)/10000 {
			return 0, false // overflow
		}
		n = n*10000 + d1*1000 + d2*100 + d3*10 + d4
		i += 4
	}

	for ; i < lenB; i++ {
		c := b[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		v := uint64(c - '0')
		if n > (math.MaxUint64-v)/10 {
			return 0, false // overflow
		}
		n = n*10 + v
	}

	if negative {
		if n > uint64(1)<<63 {
			return 0, false
		}
		if n == uint64(1)<<63 {
			return math.MinInt64, true
		}
		return -int64(n), true
	}
	if n > math.MaxInt64 {
		return 0, false
	}
	return int64(n), true
}

// parseUint64Fast parses a base-10 uint64 directly from byte slice.
// Returns (value, true) on success, (0, false) on overflow or invalid input.
// Rejects negative numbers (leading '-').
//
//go:inline
func parseUint64Fast(b []byte) (uint64, bool) {
	lenB := len(b)
	if lenB == 0 {
		return 0, false
	}

	i := 0
	if b[0] == '-' {
		return 0, false
	}
	if b[0] == '+' {
		i = 1
	}

	if i >= lenB {
		return 0, false
	}

	var n uint64
	for i+4 <= lenB {
		chunk := *(*uint32)(unsafe.Pointer(&b[i]))
		if !isAllDigits4(chunk) {
			break
		}
		d1 := uint64(b[i] - '0')
		d2 := uint64(b[i+1] - '0')
		d3 := uint64(b[i+2] - '0')
		d4 := uint64(b[i+3] - '0')
		if n > (math.MaxUint64-d1*1000-d2*100-d3*10-d4)/10000 {
			return 0, false
		}
		n = n*10000 + d1*1000 + d2*100 + d3*10 + d4
		i += 4
	}

	for ; i < lenB; i++ {
		c := b[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		v := uint64(c - '0')
		if n > (math.MaxUint64-v)/10 {
			return 0, false
		}
		n = n*10 + v
	}
	return n, true
}
