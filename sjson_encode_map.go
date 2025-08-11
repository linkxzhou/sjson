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

	// 预估缓冲区大小：每个键值对大约需要20字节（键名+引号+冒号+值+逗号）
	estimatedSize := mapLen * 20
	if cap(stream.buffer)-len(stream.buffer) < estimatedSize {
		newBuffer := make([]byte, len(stream.buffer), len(stream.buffer)+estimatedSize)
		copy(newBuffer, stream.buffer)
		stream.buffer = newBuffer
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
	elemEncoder := getEncoderFast(miValue.Type())
	err = elemEncoder.appendToBytes(stream, miValue)
	if err != nil {
		return err
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

		elemEncoder := getEncoderFast(kv.v.Type())
		err := elemEncoder.appendToBytes(stream, kv.v)
		if err != nil {
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

		miValue := mi.Value()
		elemEncoder := getEncoderFast(miValue.Type())
		err = elemEncoder.appendToBytes(stream, miValue)
		if err != nil {
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

	// 预估缓冲区大小
	estimatedSize := mapLen * 20
	if cap(stream.buffer)-len(stream.buffer) < estimatedSize {
		newBuffer := make([]byte, len(stream.buffer), len(stream.buffer)+estimatedSize)
		copy(newBuffer, stream.buffer)
		stream.buffer = newBuffer
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
