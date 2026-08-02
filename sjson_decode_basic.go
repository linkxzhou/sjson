package sjson

import (
	"encoding"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
)

var (
	trueValue  = reflect.ValueOf(true)
	falseValue = reflect.ValueOf(false)
)

// 预先缓存常用的反射类型
var (
	interfaceType = reflect.TypeOf((*interface{})(nil)).Elem()
)

// checkUnmarshaler 检查目标是否实现了 json.Unmarshaler 或 encoding.TextUnmarshaler
func (d *Decoder) checkUnmarshaler(dst reflect.Value) (bool, error) {
	// 取指针地址用于接口检查
	if !dst.CanAddr() {
		return false, nil
	}
	ptr := dst.Addr()

	// json.Unmarshaler 优先
	if u, ok := ptr.Interface().(json.Unmarshaler); ok {
		if d.token.Type == NullToken {
			return true, nil
		}
		// 读取原始 JSON 字节
		raw, err := d.readRawValue()
		if err != nil {
			return true, err
		}
		return true, u.UnmarshalJSON(raw)
	}

	// encoding.TextUnmarshaler
	if u, ok := ptr.Interface().(encoding.TextUnmarshaler); ok {
		if d.token.Type == NullToken {
			d.nextToken()
			return true, nil
		}
		if d.token.Type == StringToken {
			s := bytesToString(d.token.Value)
			d.nextToken()
			return true, u.UnmarshalText([]byte(s))
		}
	}

	return false, nil
}

// readRawValue 读取当前 token 的原始 JSON 字节（包括复合结构）
func (d *Decoder) readRawValue() ([]byte, error) {
	// 使用 token.Pos 作为起始位置（比 lexer.start 更可靠）
	start := d.token.Pos
	if start < 0 {
		start = d.lexer.start
	}
	// 先用 skipValue 跳过，但我们需要在 nextToken 之前捕获 end
	// 对于简单 token（string/number/null/bool），skipValue 调用 nextToken
	// 对于复合 token（object/array），skipObjectFast/skipArrayFast 调用 nextToken
	// 我们需要在 skipValue 之后用 lexer.pos 减去新 token 的长度

	// 更好的方案：直接在字节层面跳过，不调用 nextToken
	switch d.token.Type {
	case NullToken, TrueToken, FalseToken, IntegerToken, FloatToken, StringToken:
		end := d.lexer.pos
		d.nextToken()
		raw := make([]byte, end-start)
		copy(raw, d.lexer.input[start:end])
		return raw, nil
	case LeftBraceToken:
		return d.readRawObject()
	case LeftBracketToken:
		return d.readRawArray()
	default:
		return nil, fmt.Errorf("无法读取原始值: 未知 token %v", d.token.Type)
	}
}

// readRawObject 读取原始对象字节
func (d *Decoder) readRawObject() ([]byte, error) {
	start := d.token.Pos
	input := d.lexer.input
	pos := d.lexer.pos
	inputLen := d.lexer.inputLen
	depth := 1

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
		default:
			pos++
		}
	}

	if depth != 0 {
		return nil, fmt.Errorf("对象未正确闭合")
	}

	d.lexer.pos = pos
	d.lexer.start = pos
	d.nextToken()

	raw := make([]byte, pos-start)
	copy(raw, d.lexer.input[start:pos])
	return raw, nil
}

// readRawArray 读取原始数组字节
func (d *Decoder) readRawArray() ([]byte, error) {
	start := d.token.Pos
	input := d.lexer.input
	pos := d.lexer.pos
	inputLen := d.lexer.inputLen
	depth := 1

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
		default:
			pos++
		}
	}

	if depth != 0 {
		return nil, fmt.Errorf("数组未正确闭合")
	}

	d.lexer.pos = pos
	d.lexer.start = pos
	d.nextToken()

	raw := make([]byte, pos-start)
	copy(raw, d.lexer.input[start:pos])
	return raw, nil
}

// 解码任意值到目标反射值
func (d *Decoder) decodeValue(dst reflect.Value) error {
	if !dst.IsValid() {
		return fmt.Errorf("解码目标无效")
	}

	// 检查 json.Unmarshaler / TextUnmarshaler（在指针解引用前）
	// 对于可寻址的值，检查其指针是否实现了 Unmarshaler
	if dst.CanAddr() {
		handled, err := d.checkUnmarshaler(dst)
		if err != nil {
			return err
		}
		if handled {
			return nil
		}
	}

	// 使用循环来处理多层指针, 消除递归
	for dst.Kind() == reflect.Ptr {
		if d.token.Type == NullToken {
			// null 到指针：将指针设为 nil
			d.nextToken()
			if dst.CanSet() {
				dst.Set(reflect.Zero(dst.Type()))
			} else if !dst.IsNil() {
				// 指针不可设但非 nil（如 reflect.ValueOf 返回的指针），
				// 通过 Elem 设置指向的值为零值
				elem := dst.Elem()
				if elem.CanSet() {
					elem.Set(reflect.Zero(elem.Type()))
				}
			}
			return nil
		}
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		dst = dst.Elem()
	}

	// 再次检查 json.Unmarshaler / TextUnmarshaler（指针解引用后，此时 dst 可寻址）
	if dst.CanAddr() {
		handled, err := d.checkUnmarshaler(dst)
		if err != nil {
			return err
		}
		if handled {
			return nil
		}
	}

	// NullToken 处理（指针已解引用）
	if d.token.Type == NullToken {
		d.nextToken()
		// slice/map/interface 设为 nil（标准库语义）
		switch dst.Kind() {
		case reflect.Slice, reflect.Map, reflect.Interface:
			if dst.CanSet() {
				dst.Set(reflect.Zero(dst.Type()))
			}
			return nil
		}
		// 其他类型设置为零值
		if dst.CanSet() {
			dst.Set(reflect.Zero(dst.Type()))
		}
		return nil
	}

	// 快速路径：interface{} 类型直接处理，避免进入复杂的分支
	if dst.Kind() == reflect.Interface && dst.NumMethod() == 0 {
		return d.decodeToInterface(dst)
	}

	// 使用一个switch语句而不是多个if-else来提高性能
	switch d.token.Type {
	case TrueToken:
		d.nextToken()
		return d.decodeBool(true, dst)

	case FalseToken:
		d.nextToken()
		return d.decodeBool(false, dst)

	case StringToken:
		value := d.token.Value
		d.nextToken()
		return d.decodeString(value, dst)

	case IntegerToken, FloatToken:
		value := d.token.FloatValue
		intValue := d.token.IntValue
		raw := d.token.Value
		isInt := d.token.IsInteger
		d.nextToken()
		return d.decodeNumber(value, intValue, raw, isInt, dst)

	case LeftBraceToken:
		return d.decodeObject(dst)

	case LeftBracketToken:
		return d.decodeArray(dst)

	default:
		return fmt.Errorf("无法识别的JSON token类型: %v", d.token.Type)
	}
}

// decodeToInterface 专门优化 interface{} 类型的解码
// 避免不必要的反射操作
func (d *Decoder) decodeToInterface(dst reflect.Value) error {
	switch d.token.Type {
	case TrueToken:
		d.nextToken()
		dst.Set(trueValue)
		return nil

	case FalseToken:
		d.nextToken()
		dst.Set(falseValue)
		return nil

	case StringToken:
		value := bytesToString(d.token.Value)
		d.nextToken()
		dst.Set(reflect.ValueOf(value))
		return nil

	case IntegerToken, FloatToken:
		value := d.token.FloatValue
		d.nextToken()
		dst.Set(reflect.ValueOf(value))
		return nil

	case LeftBraceToken:
		return d.decodeInterfaceObject(dst)

	case LeftBracketToken:
		return d.decodeInterfaceArrayDirect(dst)

	case NullToken:
		d.nextToken()
		dst.Set(reflect.Zero(dst.Type()))
		return nil

	default:
		return fmt.Errorf("无法识别的JSON token类型: %v", d.token.Type)
	}
}

// decodeInterfaceObject 专门优化解码到 interface{} 的对象
func (d *Decoder) decodeInterfaceObject(dst reflect.Value) error {
	// 跳过左大括号
	d.nextToken()

	// 空对象快速路径
	if d.token.Type == RightBraceToken {
		d.nextToken()
		dst.Set(reflect.ValueOf(map[string]interface{}{}))
		return nil
	}

	// 预分配 map，估算初始容量
	m := make(map[string]interface{}, 8)

	for {
		// 键必须是字符串
		if d.token.Type != StringToken {
			return fmt.Errorf("对象键必须是字符串，得到: %v", d.token)
		}

		key := bytesToString(d.token.Value)
		d.nextToken()

		// 键后面必须是冒号
		if d.token.Type != ColonToken {
			return fmt.Errorf("对象键后面必须是冒号，得到: %v", d.token)
		}
		d.nextToken()

		// 直接解码值
		var value interface{}
		if err := d.decodeValueDirect(&value); err != nil {
			return err
		}
		m[key] = value

		// 检查分隔符
		if d.token.Type == CommaToken {
			d.nextToken()
		} else if d.token.Type == RightBraceToken {
			d.nextToken()
			break
		} else {
			return fmt.Errorf("对象中意外的标记: %v", d.token)
		}
	}

	dst.Set(reflect.ValueOf(m))
	return nil
}

// decodeInterfaceArrayDirect 专门优化解码到 interface{} 的数组
func (d *Decoder) decodeInterfaceArrayDirect(dst reflect.Value) error {
	// 跳过左方括号
	d.nextToken()

	// 空数组快速路径
	if d.token.Type == RightBracketToken {
		d.nextToken()
		dst.Set(reflect.ValueOf([]interface{}{}))
		return nil
	}

	// 从对象池获取切片
	elements := interfaceSlicePool.Get().(*[]interface{})
	*elements = (*elements)[:0]

	for {
		var element interface{}
		if err := d.decodeValueDirect(&element); err != nil {
			interfaceSlicePool.Put(elements)
			return err
		}
		*elements = append(*elements, element)

		// 检查分隔符
		if d.token.Type == CommaToken {
			d.nextToken()
		} else if d.token.Type == RightBracketToken {
			d.nextToken()
			break
		} else {
			interfaceSlicePool.Put(elements)
			return fmt.Errorf("数组中意外的标记: %v", d.token)
		}
	}

	// 复制结果并归还池
	result := make([]interface{}, len(*elements))
	copy(result, *elements)
	interfaceSlicePool.Put(elements)

	dst.Set(reflect.ValueOf(result))
	return nil
}

// decodeValueDirect 直接解码到 interface{} 指针，避免反射
func (d *Decoder) decodeValueDirect(v *interface{}) error {
	switch d.token.Type {
	case TrueToken:
		d.nextToken()
		*v = true
		return nil

	case FalseToken:
		d.nextToken()
		*v = false
		return nil

	case StringToken:
		*v = bytesToString(d.token.Value)
		d.nextToken()
		return nil

	case IntegerToken, FloatToken:
		*v = d.token.FloatValue
		d.nextToken()
		return nil

	case LeftBraceToken:
		return d.decodeObjectDirect(v)

	case LeftBracketToken:
		return d.decodeArrayDirect(v)

	case NullToken:
		d.nextToken()
		*v = nil
		return nil

	default:
		return fmt.Errorf("无法识别的JSON token类型: %v", d.token.Type)
	}
}

// decodeObjectDirect 直接解码对象到 interface{}
func (d *Decoder) decodeObjectDirect(v *interface{}) error {
	// 跳过左大括号
	d.nextToken()

	// 空对象快速路径
	if d.token.Type == RightBraceToken {
		d.nextToken()
		*v = map[string]interface{}{}
		return nil
	}

	m := make(map[string]interface{}, 8)

	for {
		if d.token.Type != StringToken {
			return fmt.Errorf("对象键必须是字符串，得到: %v", d.token)
		}

		key := bytesToString(d.token.Value)
		d.nextToken()

		if d.token.Type != ColonToken {
			return fmt.Errorf("对象键后面必须是冒号，得到: %v", d.token)
		}
		d.nextToken()

		var value interface{}
		if err := d.decodeValueDirect(&value); err != nil {
			return err
		}
		m[key] = value

		if d.token.Type == CommaToken {
			d.nextToken()
		} else if d.token.Type == RightBraceToken {
			d.nextToken()
			break
		} else {
			return fmt.Errorf("对象中意外的标记: %v", d.token)
		}
	}

	*v = m
	return nil
}

// decodeArrayDirect 直接解码数组到 interface{}
func (d *Decoder) decodeArrayDirect(v *interface{}) error {
	// 跳过左方括号
	d.nextToken()

	// 空数组快速路径
	if d.token.Type == RightBracketToken {
		d.nextToken()
		*v = []interface{}{}
		return nil
	}

	// 从对象池获取切片
	elements := interfaceSlicePool.Get().(*[]interface{})
	*elements = (*elements)[:0]

	for {
		var element interface{}
		if err := d.decodeValueDirect(&element); err != nil {
			interfaceSlicePool.Put(elements)
			return err
		}
		*elements = append(*elements, element)

		if d.token.Type == CommaToken {
			d.nextToken()
		} else if d.token.Type == RightBracketToken {
			d.nextToken()
			break
		} else {
			interfaceSlicePool.Put(elements)
			return fmt.Errorf("数组中意外的标记: %v", d.token)
		}
	}

	// 复制结果
	result := make([]interface{}, len(*elements))
	copy(result, *elements)
	interfaceSlicePool.Put(elements)

	*v = result
	return nil
}

// 解码布尔值
func (d *Decoder) decodeBool(value bool, dst reflect.Value) error {
	// 直接根据Kind处理，避免多次分支判断
	kind := dst.Kind()
	// 使用直接类型判断而非switch来减少分支
	if kind == reflect.Bool {
		dst.SetBool(value)
		return nil
	}

	if kind == reflect.Interface && dst.NumMethod() == 0 {
		if value {
			dst.Set(trueValue)
		} else {
			dst.Set(falseValue)
		}

		return nil
	}

	return fmt.Errorf("无法将布尔值解码到 %s 类型", dst.Type())
}

// 解码数字
//
// 关键优化：lexer 阶段已对有效整数计算过 IntValue，这里直接复用（isInteger=true 时），
// 命中快速路径时**不再做任何 ParseInt 调用**，避免原始实现中数字被解析两遍的问题。
// 注意：调用方在 nextToken 前捕获 value/intValue/raw/isInteger，函数内不得再读 d.token。
func (d *Decoder) decodeNumber(value float64, intValue int64, raw []byte, isInteger bool, dst reflect.Value) error {

	switch dst.Kind() {
	case reflect.Interface:
		if dst.NumMethod() == 0 {
			if isInteger && raw != nil {
				// 优先用 lexer 已算好的 IntValue（零分配）
				dst.Set(reflect.ValueOf(intValue))
				return nil
			}
			dst.Set(reflect.ValueOf(value))
			return nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if raw != nil {
			n, ok := intValue, isInteger
			if !ok {
				// 可能是浮点原始字节（如 1.5）或 int64 溢出，回退 strconv
				rawStr := bytesToString(raw)
				n2, err := strconv.ParseInt(rawStr, 10, 64)
				if err != nil {
					f, ferr := strconv.ParseFloat(rawStr, 64)
					if ferr != nil {
						return fmt.Errorf("无法将 %s 解码到 %s: %v", rawStr, dst.Type(), err)
					}
					// 拒绝小数转整数（与 encoding/json 行为一致：返回错误）
					if f != float64(int64(f)) {
						return fmt.Errorf("无法将浮点数 %s 截断为整数 %s", rawStr, dst.Type())
					}
					n2 = int64(f)
				}
				n = n2
			}
			if dst.OverflowInt(n) {
				return fmt.Errorf("数值 %d 溢出 %s", n, dst.Type())
			}
			dst.SetInt(n)
			return nil
		}
		dst.SetInt(int64(value))
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if raw != nil {
			// 先检查是否为负数
			if len(raw) > 0 && raw[0] == '-' {
				return fmt.Errorf("无法将负数 %s 解码到 %s", bytesToString(raw), dst.Type())
			}
			n, ok := uint64(intValue), isInteger
			if !ok {
				rawStr := bytesToString(raw)
				n2, err := strconv.ParseUint(rawStr, 10, 64)
				if err != nil {
					f, ferr := strconv.ParseFloat(rawStr, 64)
					if ferr != nil {
						return fmt.Errorf("无法将 %s 解码到 %s: %v", rawStr, dst.Type(), err)
					}
					if f != float64(uint64(f)) {
						return fmt.Errorf("无法将浮点数 %s 截断为无符号整数 %s", rawStr, dst.Type())
					}
					n2 = uint64(f)
				}
				n = n2
			}
			if dst.OverflowUint(n) {
				return fmt.Errorf("数值 %d 溢出 %s", n, dst.Type())
			}
			dst.SetUint(n)
			return nil
		}
		if value < 0 {
			return fmt.Errorf("无法将负数 %v 解码到 %s", value, dst.Type())
		}
		dst.SetUint(uint64(value))
		return nil
	case reflect.Float32:
		// 检查 float32 溢出
		f32 := float32(value)
		if math.IsInf(float64(f32), 0) {
			return fmt.Errorf("数值 %v 溢出 float32", value)
		}
		dst.SetFloat(float64(f32))
		return nil
	case reflect.Float64:
		dst.SetFloat(value)
		return nil
	}

	return fmt.Errorf("无法将数字解码到 %s 类型", dst.Type())
}

// 解码字符串
func (d *Decoder) decodeString(value []byte, dst reflect.Value) error {
	kind := dst.Kind()
	if kind == reflect.String {
		dst.SetString(bytesToString(value))
		return nil
	}

	if kind == reflect.Interface && dst.NumMethod() == 0 {
		dst.Set(reflect.ValueOf(bytesToString(value)))
		return nil
	}

	// 支持 []byte / []uint8：base64 解码
	if (kind == reflect.Slice || kind == reflect.Array) && dst.Type().Elem().Kind() == reflect.Uint8 {
		s := bytesToString(value)
		dec, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return fmt.Errorf("无法 base64 解码到 %s: %v", dst.Type(), err)
		}
		dst.SetBytes(dec)
		return nil
	}

	return fmt.Errorf("无法将字符串解码到 %s 类型", dst.Type())
}
