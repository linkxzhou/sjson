package sjson

import (
	"bytes"
	"encoding"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// 解码对象
func (d *Decoder) decodeObject(dst reflect.Value) error {
	// 跳过左大括号
	d.nextToken()

	kind := dst.Kind()

	// 空对象快速路径
	if d.token.Type == RightBraceToken {
		d.nextToken() // 跳过右大括号

		switch kind {
		case reflect.Map:
			// 如果是空map，创建一个空map
			if dst.IsNil() {
				dst.Set(reflect.MakeMap(dst.Type()))
			}
			return nil

		case reflect.Interface:
			if dst.NumMethod() == 0 {
				dst.Set(reflect.ValueOf(map[string]interface{}{}))
				return nil
			}
		}

		// 其他类型，空对象被忽略
		return nil
	}

	switch kind {
	case reflect.Map:
		return d.decodeMap(dst)
	case reflect.Struct:
		return d.decodeStruct(dst)
	case reflect.Interface:
		if dst.NumMethod() == 0 {
			m := make(map[string]interface{}, 8)

			// 解码到这个map
			if err := d.decodeMapStringInterface(m); err != nil {
				return err
			}

			// 设置到接口值
			dst.Set(reflect.ValueOf(m))
			return nil
		}
	}

	// 跳过整个对象
	var depth int = 1 // 已经读取了一个左大括号
	for depth > 0 && d.token.Type != EOFToken {
		if d.token.Type == LeftBraceToken {
			depth++
		} else if d.token.Type == RightBraceToken {
			depth--
		}
		d.nextToken()
	}

	return fmt.Errorf("无法将对象解码到 %s 类型", dst.Type())
}

// decodeMapStringInterface 快速解码 map[string]interface{}
func (d *Decoder) decodeMapStringInterface(m map[string]interface{}) error {
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
		switch d.consumeStructDelimiter('}') {
		case 0:
		case 1:
			goto done1
		default:
			return fmt.Errorf("对象中意外的标记: %v", d.token)
		}
	}

done1:
	return nil
}

// 解码Map
func (d *Decoder) decodeMap(dst reflect.Value) error {
	if dst.IsNil() {
		dst.Set(reflect.MakeMapWithSize(dst.Type(), 8))
	}

	elemType := dst.Type().Elem()

	keyType := dst.Type().Key()

	// 快速路径：map[string]interface{}（要求键为精确 string 类型，值为精确 interface{}）
	if keyType == exactStringType && elemType.Kind() == reflect.Interface && elemType.NumMethod() == 0 {
		m := dst.Interface().(map[string]interface{})
		return d.decodeMapStringInterface(m)
	}

	// 快速路径：map[string]string（键值都要求精确 string 类型）
	if keyType == exactStringType && elemType == exactStringType {
		return d.decodeMapStringString(dst)
	}

	// 通用路径
	for {
		// 键必须是字符串
		if d.token.Type != StringToken {
			return fmt.Errorf("对象键必须是字符串，得到: %v", d.token)
		}

		keyStr := bytesToString(d.token.Value)
		d.nextToken()

		// 键后面必须是冒号
		if d.token.Type != ColonToken {
			return fmt.Errorf("对象键后面必须是冒号，得到: %v", d.token)
		}
		d.nextToken()

		// 解码值
		valueElem := reflect.New(elemType).Elem()
		if err := d.decodeValue(valueElem); err != nil {
			return err
		}

		// 将字符串键转换为 map 的键类型
		keyType := dst.Type().Key()
		keyElem, err := convertMapKey(keyStr, keyType)
		if err != nil {
			return err
		}
		dst.SetMapIndex(keyElem, valueElem)

		// 检查是否有更多的键值对
		switch d.consumeStructDelimiter('}') {
		case 0:
		case 1:
			goto done2
		default:
			return fmt.Errorf("对象中意外的标记: %v", d.token)
		}
	}

done2:
	return nil
}

// decodeMapStringString 快速解码 map[string]string
func (d *Decoder) decodeMapStringString(dst reflect.Value) error {
	m := dst.Interface().(map[string]string)

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

		if d.token.Type != StringToken {
			return fmt.Errorf("期望字符串值，得到: %v", d.token)
		}

		m[key] = bytesToString(d.token.Value)
		d.nextToken()

		// 检查分隔符
		switch d.consumeStructDelimiter('}') {
		case 0:
		case 1:
			goto done3
		default:
			return fmt.Errorf("对象中意外的标记: %v", d.token)
		}
	}

done3:
	return nil
}

// 缓存结构体字段映射：值为 fields 切片中的下标（不是 reflect 字段索引）
var fieldMapCache = sync.Map{} // map[reflect.Type]map[string]int

// 缓存结构体字段的大小写不敏感映射，仅在精确匹配失败时作为兜底使用
// （与 encoding/json 行为一致：优先精确匹配，找不到再尝试大小写不敏感匹配）
var fieldMapCaseInsensitiveCache = sync.Map{} // map[reflect.Type]map[string]int

// 获取字段映射（带缓存），值为 fields 切片下标
func getFieldMap(structType reflect.Type, fields []structField) map[string]int {
	if cachedMap, ok := fieldMapCache.Load(structType); ok {
		return cachedMap.(map[string]int)
	}

	fieldMap := make(map[string]int, len(fields))
	for i, field := range fields {
		fieldMap[bytesToString(field.name)] = i
	}

	fieldMapCache.Store(structType, fieldMap)
	return fieldMap
}

// 获取大小写不敏感的字段映射（带缓存）
func getFieldMapCaseInsensitive(structType reflect.Type, fields []structField) map[string]int {
	if cachedMap, ok := fieldMapCaseInsensitiveCache.Load(structType); ok {
		return cachedMap.(map[string]int)
	}

	fieldMap := make(map[string]int, len(fields))
	for i, field := range fields {
		lower := strings.ToLower(bytesToString(field.name))
		// 若有冲突，保留第一个（与 encoding/json 的"第一个不区分大小写匹配"近似）
		if _, exists := fieldMap[lower]; !exists {
			fieldMap[lower] = i
		}
	}

	fieldMapCaseInsensitiveCache.Store(structType, fieldMap)
	return fieldMap
}

// 解码结构体
func (d *Decoder) decodeStruct(dst reflect.Value) error {
	structType := dst.Type()

	// 预先获取所有字段信息，避免重复查找
	fields := getStructFields(structType)

	// 小结构体（字段很少）走线性匹配：避免 sync.Map + map 哈希开销
	// 这类结构体在很多真实 payload 中占比很高（例如 BenchmarkCompareMedium）。
	useLinearScan := len(fields) <= 8

	// OPT-6: 字段数 > 8 时用预排序二分查找替代 map，避免 bytesToString 分配
	var sorted *sortedFields
	if !useLinearScan {
		sorted = getSortedFields(structType, fields)
	}

	for {
		// 键必须是字符串
		if d.token.Type != StringToken {
			return fmt.Errorf("对象键必须是字符串，得到: %v", d.token)
		}

		keyBytes := d.token.Value
		d.nextToken()

		// 键后面必须是冒号
		if d.token.Type != ColonToken {
			return fmt.Errorf("对象键后面必须是冒号，得到: %v", d.token)
		}
		d.nextToken()

		fieldPos := -1
		if useLinearScan {
			// 线性扫描：字段数很少时通常比 map 查找更快
			for i := range fields {
				if bytes.Equal(fields[i].name, keyBytes) {
					fieldPos = i
					break
				}
			}
		} else {
			// OPT-6: 二分查找，直接对 []byte 比较，零 string 分配
			fieldPos = searchFieldBinary(sorted, keyBytes)
		}

		// 精确匹配失败时，尝试大小写不敏感兜底匹配（与 encoding/json 行为一致）
		if fieldPos < 0 {
			ciMap := getFieldMapCaseInsensitive(structType, fields)
			lookupKey := bytesToString(keyBytes)
			if idx, exists := ciMap[strings.ToLower(lookupKey)]; exists {
				fieldPos = idx
			}
		}

		if fieldPos >= 0 {
			// 字段存在，解码值
			field := &fields[fieldPos]
			fv := fieldByUnsafeOffset(dst, field.offset, field.index, field.typ)
			if err := d.decodeValue(fv); err != nil {
				return fmt.Errorf("解码字段 %s 出错: %w", bytesToString(keyBytes), err)
			}
		} else {
			// 字段不存在，跳过值
			if err := d.skipValue(); err != nil {
				return err
			}
		}

		// 检查是否有更多的键值对
		switch d.consumeStructDelimiter('}') {
		case 0: // 逗号，继续
		case 1: // 右大括号，结束
			goto done4
		default:
			return fmt.Errorf("对象中意外的标记: %v", d.token)
		}
	}

done4:
	return nil
}


// convertMapKey 将字符串键转换为 map 的键类型
func convertMapKey(s string, keyType reflect.Type) (reflect.Value, error) {
	switch keyType.Kind() {
	case reflect.String:
		return reflect.ValueOf(s).Convert(keyType), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("无法将键 %q 转换为 %s: %v", s, keyType, err)
		}
		v := reflect.New(keyType).Elem()
		v.SetInt(n)
		return v, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("无法将键 %q 转换为 %s: %v", s, keyType, err)
		}
		v := reflect.New(keyType).Elem()
		v.SetUint(n)
		return v, nil
	default:
		// 尝试通过 TextUnmarshaler 接口
		v := reflect.New(keyType)
		if u, ok := v.Interface().(encoding.TextUnmarshaler); ok {
			if err := u.UnmarshalText([]byte(s)); err != nil {
				return reflect.Value{}, fmt.Errorf("无法将键 %q 通过 TextUnmarshaler 转换: %v", s, err)
			}
			return v.Elem(), nil
		}
		return reflect.Value{}, fmt.Errorf("不支持的 map 键类型: %s", keyType)
	}
}
