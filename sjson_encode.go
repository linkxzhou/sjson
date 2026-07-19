package sjson

import (
	"encoding"
	"encoding/json"
	"reflect"
	"sync"
)

// 预定义常量字符串，减少内存分配
var (
	emptyArray  = []byte("[]")
	emptyObject = []byte("{}")
	emptyString = []byte(`""`)
	nullString  = []byte("null")
	trueString  = []byte("true")
	falseString = []byte("false")
)

// 为常用类型预分配的直接编码器
var (
	// 基本类型编码器
	boolEncoderInst      = boolEncoder{}
	intEncoderInst       = intEncoder{}
	uintEncoderInst      = uintEncoder{}
	float32EncoderInst   = float32Encoder{}
	float64EncoderInst   = float64Encoder{}
	stringEncoderInst    = stringEncoder{}
	interfaceEncoderInst = interfaceEncoder{}
	defaultEncoderInst   = defaultEncoder{}
	noSupportEncoderInst = noSupportEncoder{}
)

// Encoder 是直接编码器接口，直接将Go类型编码为JSON
type Encoder interface {
	// 新增基于字节的编码方法
	appendToBytes(*encoderStream, reflect.Value) error
}

// 直接编码器缓存
var EncoderCache sync.Map // map[reflect.Type]Encoder

// 使用小对象缓存池，避免频繁创建编码器实例
var sliceEncoderPool sync.Map
var mapEncoderPool sync.Map
var ptrEncoderPool sync.Map

// json.Marshaler / encoding.TextMarshaler 接口类型，用于编码器构建时的静态检查
var (
	jsonMarshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	textMarshalerType = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
)

// jsonMarshalerEncoder 用于类型本身（值接收者或指针类型）实现 json.Marshaler 的情况
type jsonMarshalerEncoder struct{}

func (e jsonMarshalerEncoder) appendToBytes(stream *encoderStream, src reflect.Value) error {
	if src.Kind() == reflect.Ptr && src.IsNil() {
		stream.buffer = append(stream.buffer, nullString...)
		return nil
	}
	m := src.Interface().(json.Marshaler)
	data, err := m.MarshalJSON()
	if err != nil {
		return err
	}
	stream.buffer = append(stream.buffer, data...)
	return nil
}

// addrJSONMarshalerEncoder 用于仅指针接收者实现 json.Marshaler 的情况（值本身不可寻址时回退到基础编码器）
type addrJSONMarshalerEncoder struct{ fallback Encoder }

func (e addrJSONMarshalerEncoder) appendToBytes(stream *encoderStream, src reflect.Value) error {
	if src.CanAddr() {
		m := src.Addr().Interface().(json.Marshaler)
		data, err := m.MarshalJSON()
		if err != nil {
			return err
		}
		stream.buffer = append(stream.buffer, data...)
		return nil
	}
	return e.fallback.appendToBytes(stream, src)
}

// jsonTextMarshalerEncoder 用于类型本身实现 encoding.TextMarshaler 的情况
type jsonTextMarshalerEncoder struct{}

func (e jsonTextMarshalerEncoder) appendToBytes(stream *encoderStream, src reflect.Value) error {
	if src.Kind() == reflect.Ptr && src.IsNil() {
		stream.buffer = append(stream.buffer, nullString...)
		return nil
	}
	m := src.Interface().(encoding.TextMarshaler)
	data, err := m.MarshalText()
	if err != nil {
		return err
	}
	return encodeStringDirect(stream, string(data))
}

// addrJSONTextMarshalerEncoder 用于仅指针接收者实现 encoding.TextMarshaler 的情况
type addrJSONTextMarshalerEncoder struct{ fallback Encoder }

func (e addrJSONTextMarshalerEncoder) appendToBytes(stream *encoderStream, src reflect.Value) error {
	if src.CanAddr() {
		m := src.Addr().Interface().(encoding.TextMarshaler)
		data, err := m.MarshalText()
		if err != nil {
			return err
		}
		return encodeStringDirect(stream, string(data))
	}
	return e.fallback.appendToBytes(stream, src)
}

// encodeValueToBytes 直接将Go值编码到字节切片中
// json.Marshaler / encoding.TextMarshaler 的检查已下沉到 getEncoder 中静态完成，
// 这样嵌套的结构体字段、切片元素、map 值等也能正确应用自定义序列化逻辑。
func encodeValueToBytes(stream *encoderStream, src reflect.Value, typ reflect.Type) error {
	if !src.IsValid() {
		stream.buffer = append(stream.buffer, nullString...)
		return nil
	}

	encoder := getEncoder(typ)
	return encoder.appendToBytes(stream, src)
}

// 优化: 检测空值
//
//go:inline
func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	}
	return false
}

// 根据类型获取直接编码器
// 快速路径编码器获取，减少反射和缓存查找开销
func getEncoderFast(t reflect.Type) Encoder {
	// 使用原子操作的快速路径检查常用类型
	switch t.Kind() {
	case reflect.String:
		return stringEncoderInst
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return intEncoderInst
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return uintEncoderInst
	case reflect.Bool:
		return boolEncoderInst
	case reflect.Float32:
		return float32EncoderInst
	case reflect.Float64:
		return float64EncoderInst
	case reflect.Interface:
		return interfaceEncoderInst
	default:
		return getEncoder(t) // 回退到完整实现
	}
}

func getEncoder(t reflect.Type) Encoder {
	if t == nil {
		return nullEncoder{}
	}

	// 检查缓存
	if enc, ok := EncoderCache.Load(t); ok {
		return enc.(Encoder)
	}

	var enc Encoder

	// json.Marshaler / encoding.TextMarshaler 检查：
	// 类型本身或其指针类型实现了这些接口时，编码必须调用对应方法，而不能走默认反射编码
	// （time.Time 等标准库类型即依赖此机制）
	if t.Implements(jsonMarshalerType) {
		enc = jsonMarshalerEncoder{}
		EncoderCache.Store(t, enc)
		return enc
	}
	if t.Kind() != reflect.Ptr && reflect.PointerTo(t).Implements(jsonMarshalerType) {
		fallback := getEncoderBase(t)
		enc = addrJSONMarshalerEncoder{fallback: fallback}
		EncoderCache.Store(t, enc)
		return enc
	}
	if t.Implements(textMarshalerType) {
		enc = jsonTextMarshalerEncoder{}
		EncoderCache.Store(t, enc)
		return enc
	}
	if t.Kind() != reflect.Ptr && reflect.PointerTo(t).Implements(textMarshalerType) {
		fallback := getEncoderBase(t)
		enc = addrJSONTextMarshalerEncoder{fallback: fallback}
		EncoderCache.Store(t, enc)
		return enc
	}

	enc = getEncoderBase(t)
	EncoderCache.Store(t, enc)
	return enc
}

// getEncoderBase 构建不考虑 Marshaler 接口的基础编码器（内部使用，避免递归检查接口）
func getEncoderBase(t reflect.Type) Encoder {
	var enc Encoder

	// 使用预分配的基本类型编码器实例
	switch t.Kind() {
	case reflect.Bool:
		enc = boolEncoderInst
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		enc = intEncoderInst
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		enc = uintEncoderInst
	case reflect.Float32:
		enc = float32EncoderInst
	case reflect.Float64:
		enc = float64EncoderInst
	case reflect.String:
		enc = stringEncoderInst
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			// []byte 特殊处理为字符串
			enc = byteSliceEncoder{}
		} else {
			// 检查是否有缓存的 sliceEncoder
			if cachedEnc, ok := sliceEncoderPool.Load(t.Elem()); ok {
				enc = cachedEnc.(Encoder)
			} else {
				sliceEnc := sliceEncoder{elemType: t.Elem()}
				sliceEncoderPool.Store(t.Elem(), sliceEnc)
				enc = sliceEnc
			}
		}
	case reflect.Map:
		// 针对 map[string]interface{} 类型优化
		switch t.Key().Kind() {
		case reflect.String,
			reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if t.Elem().Kind() == reflect.Interface {
				enc = mapStringInterfaceEncoder{
					keyType:   t.Key(),
					valueType: t.Elem(),
				}
			} else {
				if cachedEnc, ok := mapEncoderPool.Load(t.Elem()); ok {
					enc = cachedEnc.(Encoder)
				} else {
					mapEnc := mapEncoder{
						keyType:      t.Key(),
						valueType:    t.Elem(),
						valueEncoder: getEncoder(t.Elem()),
					}
					mapEncoderPool.Store(t.Elem(), mapEnc)
					enc = mapEnc
				}
			}
		default:
			enc = noSupportEncoderInst
		}
	case reflect.Struct:
		// 结构体编码器优化，预缓存字段信息
		fields := getStructFields(t)

		// 统计字段信息用于优化
		hasOmitEmpty := false
		for _, field := range fields {
			if field.omitempty {
				hasOmitEmpty = true
				break
			}
		}

		enc = &structEncoder{
			typ:          t,
			fields:       fields,
			numFields:    len(fields),
			hasOmitEmpty: hasOmitEmpty,
			opcodes:      newStructOpcodeProgram(t, fields),
		}
	case reflect.Interface:
		enc = interfaceEncoderInst
	case reflect.Ptr:
		// 检查是否有缓存的 ptrEncoder
		if cachedEnc, ok := ptrEncoderPool.Load(t.Elem()); ok {
			enc = cachedEnc.(Encoder)
		} else {
			ptrEnc := ptrEncoder{elemType: t.Elem()}
			ptrEncoderPool.Store(t.Elem(), ptrEnc)
			enc = ptrEnc
		}
	default:
		enc = defaultEncoderInst
	}

	return enc
}
