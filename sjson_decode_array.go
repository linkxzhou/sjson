package sjson

import (
	"fmt"
	"reflect"
)

// 解码数组
func (d *Decoder) decodeArray(dst reflect.Value) error {
	// 跳过左方括号
	d.nextToken()

	kind := dst.Kind()

	// 空数组快速路径
	if d.token.Type == RightBracketToken {
		d.nextToken() // 跳过右方括号

		switch kind {
		case reflect.Slice:
			// 创建空切片
			dst.Set(reflect.MakeSlice(dst.Type(), 0, 0))
		case reflect.Interface:
			if dst.NumMethod() == 0 {
				// 创建空数组并设置到接口
				dst.Set(reflect.ValueOf([]interface{}{}))
			}
		}
		return nil
	}

	switch kind {
	case reflect.Slice:
		return d.decodeSlice(dst)
	case reflect.Array:
		return d.decodeFixedArray(dst)
	case reflect.Interface:
		if dst.NumMethod() == 0 {
			return d.decodeInterfaceArray(dst)
		}
	}

	// 跳过整个数组
	var depth int = 1 // 已经读取了一个左方括号
	for depth > 0 && d.token.Type != EOFToken {
		if d.token.Type == LeftBracketToken {
			depth++
		} else if d.token.Type == RightBracketToken {
			depth--
		}
		d.nextToken()
	}

	return fmt.Errorf("无法将数组解码到 %s 类型", dst.Type())
}

// 预定义的精确类型，用于快速路径的类型安全检查（避免命名类型如 type MyInt int 误入快路径）
var (
	exactIntType     = reflect.TypeOf(0)
	exactStringType  = reflect.TypeOf("")
	exactFloat64Type = reflect.TypeOf(float64(0))
)

// 解码到切片
func (d *Decoder) decodeSlice(dst reflect.Value) error {
	elemType := dst.Type().Elem()

	// 快速路径：[]interface{} 类型（要求精确的 interface{}，无自定义方法）
	if elemType.Kind() == reflect.Interface && elemType.NumMethod() == 0 {
		return d.decodeInterfaceSliceFast(dst)
	}

	// 快速路径：[]int 类型（精确类型匹配，排除 type MyInt int 等命名类型）
	if elemType == exactIntType {
		return d.decodeIntSlice(dst)
	}

	// 快速路径：[]string 类型（精确类型匹配）
	if elemType == exactStringType {
		return d.decodeStringSlice(dst)
	}

	// 快速路径：[]float64 类型（精确类型匹配）
	if elemType == exactFloat64Type {
		return d.decodeFloat64Slice(dst)
	}

	// 快速路径：[]*struct（常见场景，避免 reflect.New/MakeSlice 的反复分配）
	if elemType.Kind() == reflect.Ptr && elemType.Elem().Kind() == reflect.Struct {
		return d.decodePtrStructSlice(dst, elemType)
	}

	// 通用路径
	return d.decodeSliceGeneric(dst, elemType)
}

// decodeInterfaceSliceFast 快速解码 []interface{}
func (d *Decoder) decodeInterfaceSliceFast(dst reflect.Value) error {
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
		switch d.consumeStructDelimiter(']') {
		case 0:
		case 1:
			goto done1
		default:
			interfaceSlicePool.Put(elements)
			return fmt.Errorf("数组中意外的标记: %v", d.token)
		}
	}

done1:
	// 复制结果
	result := make([]interface{}, len(*elements))
	copy(result, *elements)
	interfaceSlicePool.Put(elements)

	dst.Set(reflect.ValueOf(result))
	return nil
}

// decodeIntSlice 快速解码 []int
func (d *Decoder) decodeIntSlice(dst reflect.Value) error {
	// 预分配切片
	result := make([]int, 0, 8)

	for {
		// 检查是否是数字
		if d.token.Type != IntegerToken && d.token.Type != FloatToken {
			return fmt.Errorf("期望数字，得到: %v", d.token)
		}

		// 优先使用 IntValue（零分配），未设置时才用 FloatValue
		var n int
		if d.token.Type == IntegerToken && d.token.IsInteger {
			n = int(d.token.IntValue)
		} else {
			n = int(d.token.FloatValue)
		}
		result = append(result, n)
		d.nextToken()

		// 检查分隔符
		switch d.consumeStructDelimiter(']') {
		case 0:
		case 1:
			goto done
		default:
			return fmt.Errorf("数组中意外的标记: %v", d.token)
		}
	}

done:
	dst.Set(reflect.ValueOf(result))
	return nil
}

// decodeStringSlice 快速解码 []string
func (d *Decoder) decodeStringSlice(dst reflect.Value) error {
	result := make([]string, 0, 8)

	for {
		if d.token.Type != StringToken {
			return fmt.Errorf("期望字符串，得到: %v", d.token)
		}

		result = append(result, bytesToString(d.token.Value))
		d.nextToken()

		// 检查分隔符
		switch d.consumeStructDelimiter(']') {
		case 0:
		case 1:
			goto done2
		default:
			return fmt.Errorf("数组中意外的标记: %v", d.token)
		}
	}

done2:
	dst.Set(reflect.ValueOf(result))
	return nil
}

// decodeFloat64Slice 快速解码 []float64
func (d *Decoder) decodeFloat64Slice(dst reflect.Value) error {
	result := make([]float64, 0, 8)

	for {
		if d.token.Type != IntegerToken && d.token.Type != FloatToken {
			return fmt.Errorf("期望数字，得到: %v", d.token)
		}

		result = append(result, d.token.FloatValue)
		d.nextToken()

		// 检查分隔符
		switch d.consumeStructDelimiter(']') {
		case 0:
		case 1:
			goto done3
		default:
			return fmt.Errorf("数组中意外的标记: %v", d.token)
		}
	}

done3:
	dst.Set(reflect.ValueOf(result))
	return nil
}

// decodePtrStructSlice 快速解码 []*T（T 为 struct），复用 slice 容量与元素指针
func (d *Decoder) decodePtrStructSlice(dst reflect.Value, elemType reflect.Type) error {
	// 初始化/复用切片
	if dst.IsNil() {
		dst.Set(reflect.MakeSlice(dst.Type(), 0, 8))
	}

	oldLen := dst.Len()
	n := 0

	for {
		// 允许数组元素为 null
		switch d.token.Type {
		case NullToken:
			// 将该元素置为 nil
			if n < dst.Len() {
				// 确保长度至少为 n+1
				if n >= dst.Len() {
					dst.SetLen(n + 1)
				}
				idx := dst.Index(n)
				idx.Set(reflect.Zero(elemType))
			} else {
				// 需要扩容/追加
				if n < dst.Cap() {
					dst.SetLen(n + 1)
					dst.Index(n).Set(reflect.Zero(elemType))
				} else {
					dst.Set(reflect.Append(dst, reflect.Zero(elemType)))
				}
			}
			d.nextToken()

		case LeftBraceToken:
			// 确保切片长度至少为 n+1
			if n >= dst.Len() {
				if n < dst.Cap() {
					dst.SetLen(n + 1)
				} else {
					dst.Set(reflect.Append(dst, reflect.Zero(elemType)))
				}
			}

			elemPtr := dst.Index(n)
			if elemPtr.IsNil() {
				elemPtr.Set(reflect.New(elemType.Elem()))
			}

			// 直接解码对象到结构体，避免 decodeValue 的指针展开开销
			if err := d.decodeObject(elemPtr.Elem()); err != nil {
				return err
			}

		default:
			return fmt.Errorf("期望对象或null，得到: %v", d.token)
		}

		n++

		// 检查分隔符
		switch d.consumeStructDelimiter(']') {
		case 0:
			continue
		case 1:
			goto done4
		default:
			return fmt.Errorf("数组中意外的标记: %v", d.token)
		}
	}

done4:
	// 如果比原来短，清理尾部以避免保留旧指针
	if oldLen > n {
		for i := n; i < oldLen; i++ {
			dst.Index(i).Set(reflect.Zero(elemType))
		}
	}

	dst.SetLen(n)
	return nil
}

// decodeSliceGeneric 通用切片解码
func (d *Decoder) decodeSliceGeneric(dst reflect.Value, elemType reflect.Type) error {
	// 从对象池获取切片
	elemValues := valueSlicePool.Get().(*[]reflect.Value)
	*elemValues = (*elemValues)[:0] // 清空但保留容量
	defer valueSlicePool.Put(elemValues)

	// 收集元素
	for {
		// 解码值
		elem := reflect.New(elemType).Elem()
		if err := d.decodeValue(elem); err != nil {
			return err
		}
		*elemValues = append(*elemValues, elem)

		// 检查分隔符
		switch d.consumeStructDelimiter(']') {
		case 0:
		case 1:
			goto done5
		default:
			return fmt.Errorf("数组中意外的标记: %v", d.token)
		}
	}

done5:
	// 创建最终切片
	length := len(*elemValues)
	sliceValue := reflect.MakeSlice(dst.Type(), length, length)

	// 复制元素到最终切片
	for i, elem := range *elemValues {
		sliceValue.Index(i).Set(elem)
	}

	dst.Set(sliceValue)
	return nil
}

// 解码到固定数组
func (d *Decoder) decodeFixedArray(dst reflect.Value) error {
	// 不需要额外的内存分配，直接解码到目标数组
	arrayLen := dst.Len()

	for i := 0; i < arrayLen; i++ {
		// 如果JSON数组结束，跳出循环
		if d.token.Type == RightBracketToken {
			d.nextToken() // 跳过右方括号
			return nil
		}

		// 解码到数组元素
		if err := d.decodeValue(dst.Index(i)); err != nil {
			return err
		}

		// 检查分隔符
		// OPT-2: 使用 peekByte 快速检测
		switch d.consumeStructDelimiter(']') {
		case 0:
		case 1:
			return nil
		default:
			return fmt.Errorf("数组中意外的标记: %v", d.token)
		}
	}

	// 如果JSON数组元素多于Go数组长度，跳过多余元素
	for d.token.Type != RightBracketToken && d.token.Type != EOFToken {
		if err := d.skipValue(); err != nil {
			return err
		}

		// 检查分隔符
		// OPT-2: 使用 peekByte 快速检测
		switch d.consumeStructDelimiter(']') {
		case 0:
		case 1:
			return nil
		default:
			return fmt.Errorf("数组中意外的标记: %v", d.token)
		}
	}

	return nil
}

// 解码到接口数组
func (d *Decoder) decodeInterfaceArray(dst reflect.Value) error {
	// 从对象池获取接口切片
	elements := interfaceSlicePool.Get().(*[]interface{})
	*elements = (*elements)[:0] // 清空但保留容量

	// 解析元素
	for {
		var element interface{}
		if err := d.decodeValueDirect(&element); err != nil {
			interfaceSlicePool.Put(elements)
			return err
		}
		*elements = append(*elements, element)

		// 检查分隔符
		switch d.consumeStructDelimiter(']') {
		case 0:
		case 1:
			goto done6
		default:
			interfaceSlicePool.Put(elements)
			return fmt.Errorf("数组中意外的标记: %v", d.token)
		}
	}

done6:
	// 复制结果
	result := make([]interface{}, len(*elements))
	copy(result, *elements)
	interfaceSlicePool.Put(elements)

	dst.Set(reflect.ValueOf(result))
	return nil
}
