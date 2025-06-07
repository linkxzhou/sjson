package sjson

import (
	"fmt"
	"reflect"
	"strconv"
)

// 各种类型的直接编码器实现
type nullEncoder struct{}

//go:inline
//go:nosplit
func (e nullEncoder) appendToBytes(stream *encoderStream, _ reflect.Value) error {
	stream.buffer = append(stream.buffer, nullString...)
	return nil
}

type boolEncoder struct{}

//go:inline
//go:nosplit
func (e boolEncoder) appendToBytes(stream *encoderStream, src reflect.Value) error {
	if src.Bool() {
		stream.buffer = append(stream.buffer, trueString...)
	} else {
		stream.buffer = append(stream.buffer, falseString...)
	}
	return nil
}

type intEncoder struct{}

//go:inline
//go:nosplit
func (e intEncoder) appendToBytes(stream *encoderStream, src reflect.Value) error {
	intValue := src.Int()
	stream.buffer = appendInt(stream.buffer, intValue, 10)
	return nil
}

type uintEncoder struct{}

//go:inline
//go:nosplit
func (e uintEncoder) appendToBytes(stream *encoderStream, src reflect.Value) error {
	stream.buffer = appendUint(stream.buffer, src.Uint(), 10)
	return nil
}

type float32Encoder struct{}

//go:inline
func (e float32Encoder) appendToBytes(stream *encoderStream, src reflect.Value) error {
	f := float32(src.Float())
	return appendFloat32(stream, f)
}

type float64Encoder struct{}

//go:inline
func (e float64Encoder) appendToBytes(stream *encoderStream, src reflect.Value) error {
	f := src.Float()
	return appendFloat64(stream, f)
}

// 优化的浮点数编码函数，参考 jsoniter 的实现
//
//go:inline
func appendFloat32(stream *encoderStream, f float32) error {

	// 检查是否为整数浮点数
	if f == float32(int32(f)) && f >= -2147483648 && f <= 2147483647 {
		stream.buffer = appendInt(stream.buffer, int64(f), 10)
		return nil
	}

	// 使用 6 位精度进行快速编码（参考 jsoniter ConfigFastest）
	stream.buffer = strconv.AppendFloat(stream.buffer, float64(f), 'g', 6, 32)
	return nil
}

//go:inline
func appendFloat64(stream *encoderStream, f float64) error {
	// 检查是否为整数浮点数
	if f == float64(int64(f)) && f >= -9223372036854775808 && f <= 9223372036854775807 {
		stream.buffer = appendInt(stream.buffer, int64(f), 10)
		return nil
	}

	// 使用 6 位精度进行快速编码（参考 jsoniter ConfigFastest）
	stream.buffer = strconv.AppendFloat(stream.buffer, f, 'g', 6, 64)
	return nil
}

type defaultEncoder struct{}

func (e defaultEncoder) appendToBytes(stream *encoderStream, src reflect.Value) error {
	// 默认策略：转换为字符串返回
	return stringEncoderInst.appendToBytes(stream, src)
}

type noSupportEncoder struct{}

func (e noSupportEncoder) appendToBytes(stream *encoderStream, src reflect.Value) error {
	return fmt.Errorf("unsupported map key type: %v", src.Type())
}
