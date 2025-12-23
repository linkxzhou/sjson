package sjson

import (
	"fmt"
	"reflect"
)

var (
	trueValue  = reflect.ValueOf(true)
	falseValue = reflect.ValueOf(false)
)

// 预先缓存常用的反射类型
var (
	interfaceType = reflect.TypeOf((*interface{})(nil)).Elem()
)

// 解码任意值到目标反射值
func (d *Decoder) decodeValue(dst reflect.Value) error {
	if !dst.IsValid() {
		return fmt.Errorf("解码目标无效")
	}

	// 使用循环来处理多层指针, 消除递归
	for dst.Kind() == reflect.Ptr {
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		dst = dst.Elem()
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
		d.nextToken()
		return d.decodeNumber(value, dst)

	case LeftBraceToken:
		return d.decodeObject(dst)

	case LeftBracketToken:
		return d.decodeArray(dst)

	case NullToken:
		d.nextToken()
		// 对于null值，设置为零值
		dst.Set(reflect.Zero(dst.Type()))
		return nil

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
func (d *Decoder) decodeNumber(value float64, dst reflect.Value) error {
	// 根据当前值的类型优化解析路径
	switch dst.Kind() {
	case reflect.Interface:
		if dst.NumMethod() == 0 {
			dst.Set(reflect.ValueOf(value))
			return nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		dst.SetInt(int64(value))
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		dst.SetUint(uint64(value))
		return nil
	case reflect.Float32, reflect.Float64:
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

	return fmt.Errorf("无法将字符串解码到 %s 类型", dst.Type())
}
