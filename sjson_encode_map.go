package sjson

import (
	"bytes"
	"encoding"
	"fmt"
	"reflect"
	"slices"
	"sync"
)

// 对象池优化：复用 reflectWithString 切片
var reflectWithStringPool = sync.Pool{
	New: func() interface{} {
		return make([]reflectWithString, 0, 16)
	},
}

func getReflectWithStringSlice(size int) []reflectWithString {
	slice := reflectWithStringPool.Get().([]reflectWithString)
	if cap(slice) < size {
		return make([]reflectWithString, 0, size)
	}
	return slice[:0]
}

func putReflectWithStringSlice(slice []reflectWithString) {
	if cap(slice) <= 64 { // 避免池中对象过大
		reflectWithStringPool.Put(slice)
	}
}

// map[string]interface{} 专用编码器
type mapStringInterfaceEncoder struct {
	keyType   reflect.Type
	valueType reflect.Type
}

// 为 mapStringInterfaceEncoder 添加 appendToBytes 方法
func (e mapStringInterfaceEncoder) appendToBytes(stream *encoderStream, src reflect.Value) error {
	if src.IsNil() {
		stream.buffer = append(stream.buffer, nullString...)
		return nil
	}

	mapLen := src.Len()
	if mapLen == 0 {
		stream.buffer = append(stream.buffer, emptyObject...)
		return nil
	}

	// 开始构建JSON对象
	stream.buffer = append(stream.buffer, '{')

	var mi = src.MapRange()

	// 根据map大小选择不同的编码策略
	if mapLen == 1 {
		return e.encodeSinglePair(stream, mi)
	}

	return e.encodeMultiplePairs(stream, mi, mapLen)

}

// 编码单个键值对（优化路径）
func (e mapStringInterfaceEncoder) encodeSinglePair(stream *encoderStream, mi *reflect.MapIter) error {
	mi.Next()
	ks, err := resolveKeyName(mi.Key())
	if err != nil {
		return fmt.Errorf("json: encoding error for map key: %q", err.Error())
	}

	stream.buffer = append(stream.buffer, '"')
	stream.buffer = append(stream.buffer, ks...)
	stream.buffer = append(stream.buffer, '"', ':')

	miValue := mi.Value()

	// 快速路径：直接处理常见interface{}内部类型
	if err := encodeInterfaceValueFast(stream, miValue); err != nil {
		return err
	}

	stream.buffer = append(stream.buffer, '}')
	return nil
}

// encodeInterfaceValueFast 快速编码interface{}值
func encodeInterfaceValueFast(stream *encoderStream, v reflect.Value) error {
	if v.IsNil() {
		stream.buffer = append(stream.buffer, nullString...)
		return nil
	}

	elem := v.Elem()
	switch elem.Kind() {
	case reflect.String:
		return encodeStringDirect(stream, elem.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		stream.buffer = appendInt(stream.buffer, elem.Int(), 10)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		stream.buffer = appendUint(stream.buffer, elem.Uint(), 10)
		return nil
	case reflect.Float64:
		return appendFloat64(stream, elem.Float())
	case reflect.Float32:
		return appendFloat32(stream, float32(elem.Float()))
	case reflect.Bool:
		if elem.Bool() {
			stream.buffer = append(stream.buffer, trueString...)
		} else {
			stream.buffer = append(stream.buffer, falseString...)
		}
		return nil
	case reflect.Slice:
		if elem.IsNil() {
			stream.buffer = append(stream.buffer, nullString...)
			return nil
		}
		// 检查是否是 []interface{}
		if elem.Type().Elem().Kind() == reflect.Interface {
			return encodeInterfaceSliceFast(stream, elem)
		}
		// 检查是否是 []string
		if elem.Type().Elem().Kind() == reflect.String {
			return encodeStringSliceFast(stream, elem)
		}
		// 检查是否是 []int
		if elem.Type().Elem().Kind() == reflect.Int {
			return encodeIntSliceFast(stream, elem)
		}
	case reflect.Map:
		if elem.IsNil() {
			stream.buffer = append(stream.buffer, nullString...)
			return nil
		}
		// 检查是否是 map[string]interface{}
		if elem.Type().Key().Kind() == reflect.String && elem.Type().Elem().Kind() == reflect.Interface {
			return encodeMapStringInterfaceFast(stream, elem)
		}
	}

	// 回退到通用编码器
	elemEncoder := getEncoderFast(elem.Type())
	return elemEncoder.appendToBytes(stream, elem)
}

// encodeInterfaceSliceFast 快速编码 []interface{}
func encodeInterfaceSliceFast(stream *encoderStream, src reflect.Value) error {
	length := src.Len()
	if length == 0 {
		stream.buffer = append(stream.buffer, emptyArray...)
		return nil
	}

	stream.buffer = append(stream.buffer, '[')

	for i := 0; i < length; i++ {
		if i > 0 {
			stream.buffer = append(stream.buffer, ',')
		}
		if err := encodeInterfaceValueFast(stream, src.Index(i)); err != nil {
			return err
		}
	}

	stream.buffer = append(stream.buffer, ']')
	return nil
}

// encodeStringSliceFast 快速编码 []string
func encodeStringSliceFast(stream *encoderStream, src reflect.Value) error {
	length := src.Len()
	if length == 0 {
		stream.buffer = append(stream.buffer, emptyArray...)
		return nil
	}

	stream.buffer = append(stream.buffer, '[')

	for i := 0; i < length; i++ {
		if i > 0 {
			stream.buffer = append(stream.buffer, ',')
		}
		if err := encodeStringDirect(stream, src.Index(i).String()); err != nil {
			return err
		}
	}

	stream.buffer = append(stream.buffer, ']')
	return nil
}

// encodeIntSliceFast 快速编码 []int
func encodeIntSliceFast(stream *encoderStream, src reflect.Value) error {
	length := src.Len()
	if length == 0 {
		stream.buffer = append(stream.buffer, emptyArray...)
		return nil
	}

	stream.buffer = append(stream.buffer, '[')

	for i := 0; i < length; i++ {
		if i > 0 {
			stream.buffer = append(stream.buffer, ',')
		}
		stream.buffer = appendInt(stream.buffer, src.Index(i).Int(), 10)
	}

	stream.buffer = append(stream.buffer, ']')
	return nil
}

// encodeMapStringInterfaceFast 快速编码 map[string]interface{}
func encodeMapStringInterfaceFast(stream *encoderStream, src reflect.Value) error {
	mapLen := src.Len()
	if mapLen == 0 {
		stream.buffer = append(stream.buffer, emptyObject...)
		return nil
	}

	stream.buffer = append(stream.buffer, '{')

	mi := src.MapRange()
	first := true
	for mi.Next() {
		if !first {
			stream.buffer = append(stream.buffer, ',')
		}
		first = false

		// 编码键
		stream.buffer = append(stream.buffer, '"')
		stream.buffer = append(stream.buffer, mi.Key().String()...)
		stream.buffer = append(stream.buffer, '"', ':')

		// 编码值
		if err := encodeInterfaceValueFast(stream, mi.Value()); err != nil {
			return err
		}
	}

	stream.buffer = append(stream.buffer, '}')
	return nil
}

// 编码多个键值对
func (e mapStringInterfaceEncoder) encodeMultiplePairs(stream *encoderStream, mi *reflect.MapIter, mapLen int) error {
	if defaultConfig.SortMapKeys {
		return e.encodeSortedPairs(stream, mi, mapLen)
	}
	return e.encodeUnsortedPairs(stream, mi)
}

// 编码排序的键值对
func (e mapStringInterfaceEncoder) encodeSortedPairs(stream *encoderStream, mi *reflect.MapIter, mapLen int) error {
	sv := getReflectWithStringSlice(mapLen)
	defer putReflectWithStringSlice(sv)

	// 确保切片有足够容量
	if cap(sv) < mapLen {
		sv = make([]reflectWithString, mapLen)
	} else {
		sv = sv[:mapLen]
	}

	for i := 0; mi.Next(); i++ {
		ks, err := resolveKeyName(mi.Key())
		if err != nil {
			return fmt.Errorf("json: encoding error for map key: %q", err.Error())
		}
		sv[i].ks = ks
		sv[i].v = mi.Value()
	}

	slices.SortFunc(sv, func(i, j reflectWithString) int {
		return bytes.Compare(i.ks, j.ks)
	})

	for i, kv := range sv {
		if i > 0 {
			stream.buffer = append(stream.buffer, ',')
		}
		stream.buffer = append(stream.buffer, '"')
		stream.buffer = append(stream.buffer, kv.ks...)
		stream.buffer = append(stream.buffer, '"', ':')

		if err := encodeInterfaceValueFast(stream, kv.v); err != nil {
			return err
		}
	}

	stream.buffer = append(stream.buffer, '}')
	return nil
}

// 编码未排序的键值对（快速路径）
func (e mapStringInterfaceEncoder) encodeUnsortedPairs(stream *encoderStream, mi *reflect.MapIter) error {
	for i := 0; mi.Next(); i++ {
		ks, err := resolveKeyName(mi.Key())
		if err != nil {
			return fmt.Errorf("json: encoding error for map key: %q", err.Error())
		}

		if i > 0 {
			stream.buffer = append(stream.buffer, ',')
		}
		stream.buffer = append(stream.buffer, '"')
		stream.buffer = append(stream.buffer, ks...)
		stream.buffer = append(stream.buffer, '"', ':')

		if err := encodeInterfaceValueFast(stream, mi.Value()); err != nil {
			return err
		}
	}

	stream.buffer = append(stream.buffer, '}')
	return nil
}

type mapEncoder struct {
	keyType      reflect.Type
	valueType    reflect.Type
	valueEncoder Encoder // 预缓存值编码器
}

// 为 mapEncoder 添加 appendToBytes 方法
func (e mapEncoder) appendToBytes(stream *encoderStream, src reflect.Value) error {
	if src.IsNil() {
		stream.buffer = append(stream.buffer, nullString...)
		return nil
	}

	mapLen := src.Len()
	if mapLen == 0 {
		stream.buffer = append(stream.buffer, emptyObject...)
		return nil
	}

	// 开始构建JSON对象
	stream.buffer = append(stream.buffer, '{')

	var mi = src.MapRange()

	// 根据map大小选择不同的编码策略
	if mapLen == 1 {
		return e.encodeSinglePair(stream, mi)
	}

	return e.encodeMultiplePairs(stream, mi, mapLen)

}

// 编码单个键值对（优化路径）
func (e mapEncoder) encodeSinglePair(stream *encoderStream, mi *reflect.MapIter) error {
	mi.Next()
	ks, err := resolveKeyName(mi.Key())
	if err != nil {
		return fmt.Errorf("json: encoding error for map key: %q", err.Error())
	}

	stream.buffer = append(stream.buffer, '"')
	stream.buffer = append(stream.buffer, ks...)
	stream.buffer = append(stream.buffer, '"', ':')

	err = e.valueEncoder.appendToBytes(stream, mi.Value())
	if err != nil {
		return err
	}

	stream.buffer = append(stream.buffer, '}')
	return nil
}

// 编码多个键值对
func (e mapEncoder) encodeMultiplePairs(stream *encoderStream, mi *reflect.MapIter, mapLen int) error {
	if defaultConfig.SortMapKeys {
		return e.encodeSortedPairs(stream, mi, mapLen)
	}
	return e.encodeUnsortedPairs(stream, mi)
}

// 编码排序的键值对
func (e mapEncoder) encodeSortedPairs(stream *encoderStream, mi *reflect.MapIter, mapLen int) error {
	sv := getReflectWithStringSlice(mapLen)
	defer putReflectWithStringSlice(sv)

	// 确保切片有足够容量
	if cap(sv) < mapLen {
		sv = make([]reflectWithString, mapLen)
	} else {
		sv = sv[:mapLen]
	}

	for i := 0; mi.Next(); i++ {
		ks, err := resolveKeyName(mi.Key())
		if err != nil {
			return fmt.Errorf("json: encoding error for map key: %q", err.Error())
		}
		sv[i].ks = ks
		sv[i].v = mi.Value()
	}

	slices.SortFunc(sv, func(i, j reflectWithString) int {
		return bytes.Compare(i.ks, j.ks)
	})

	for i, kv := range sv {
		if i > 0 {
			stream.buffer = append(stream.buffer, ',')
		}
		stream.buffer = append(stream.buffer, '"')
		stream.buffer = append(stream.buffer, kv.ks...)
		stream.buffer = append(stream.buffer, '"', ':')

		err := e.valueEncoder.appendToBytes(stream, kv.v)
		if err != nil {
			return err
		}
	}

	stream.buffer = append(stream.buffer, '}')
	return nil
}

// 编码未排序的键值对（快速路径）
func (e mapEncoder) encodeUnsortedPairs(stream *encoderStream, mi *reflect.MapIter) error {
	for i := 0; mi.Next(); i++ {
		ks, err := resolveKeyName(mi.Key())
		if err != nil {
			return fmt.Errorf("json: encoding error for map key: %q", err.Error())
		}

		if i > 0 {
			stream.buffer = append(stream.buffer, ',')
		}
		stream.buffer = append(stream.buffer, '"')
		stream.buffer = append(stream.buffer, ks...)
		stream.buffer = append(stream.buffer, '"', ':')

		err = e.valueEncoder.appendToBytes(stream, mi.Value())
		if err != nil {
			return err
		}
	}

	stream.buffer = append(stream.buffer, '}')
	return nil
}

type reflectWithString struct {
	v  reflect.Value
	ks []byte
}

//go:inline
func resolveKeyName(src reflect.Value) ([]byte, error) {
	if src.Kind() == reflect.String {
		return stringToBytes(src.String()), nil
	}

	if tm, ok := src.Interface().(encoding.TextMarshaler); ok {
		if src.Kind() == reflect.Pointer && src.IsNil() {
			return emptyString, nil
		}
		return tm.MarshalText()
	}

	switch src.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return appendInt(nil, src.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return appendUint(nil, src.Uint(), 10), nil
	}

	return nil, fmt.Errorf("unexpected map key type: %v", src.Type())
}
