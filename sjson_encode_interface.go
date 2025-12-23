package sjson

import (
	"reflect"
)

type sliceEncoder struct {
	elemType reflect.Type
}

func (e sliceEncoder) appendToBytes(stream *encoderStream, src reflect.Value) error {
	if src.IsNil() {
		stream.buffer = append(stream.buffer, nullString...)
		return nil
	}

	length := src.Len()
	if length == 0 {
		stream.buffer = append(stream.buffer, emptyArray...)
		return nil
	}

	// 快速路径：[]int
	if e.elemType.Kind() == reflect.Int {
		return encodeIntSliceFast(stream, src)
	}

	// 快速路径：[]string
	if e.elemType.Kind() == reflect.String {
		return encodeStringSliceFast(stream, src)
	}

	// 快速路径：[]float64
	if e.elemType.Kind() == reflect.Float64 {
		return encodeFloat64SliceFastImpl(stream, src)
	}

	stream.buffer = append(stream.buffer, '[')

	// 获取元素的编码器
	elemEncoder := getEncoder(e.elemType)

	var err error

	// 编码剩余元素
	for i := 0; i < length; i++ {
		if i > 0 {
			stream.buffer = append(stream.buffer, ',')
		}
		err = elemEncoder.appendToBytes(stream, src.Index(i))
		if err != nil {
			return err
		}
	}

	stream.buffer = append(stream.buffer, ']')
	return nil
}

// encodeFloat64SliceFastImpl 快速编码 []float64
func encodeFloat64SliceFastImpl(stream *encoderStream, src reflect.Value) error {
	length := src.Len()
	stream.buffer = append(stream.buffer, '[')

	for i := 0; i < length; i++ {
		if i > 0 {
			stream.buffer = append(stream.buffer, ',')
		}
		if err := appendFloat64(stream, src.Index(i).Float()); err != nil {
			return err
		}
	}

	stream.buffer = append(stream.buffer, ']')
	return nil
}

type interfaceEncoder struct{}

func (e interfaceEncoder) appendToBytes(stream *encoderStream, src reflect.Value) error {
	if src.IsNil() {
		stream.buffer = append(stream.buffer, nullString...)
		return nil
	}

	// 获取接口中实际的值
	elem := src.Elem()

	// 快速路径：直接处理常见类型，避免getEncoder调用
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
	}

	// 获取元素的编码器
	elemEncoder := getEncoder(elem.Type())
	return elemEncoder.appendToBytes(stream, elem)
}

type ptrEncoder struct {
	elemType reflect.Type
}

func (e ptrEncoder) appendToBytes(stream *encoderStream, src reflect.Value) error {
	if src.IsNil() {
		stream.buffer = append(stream.buffer, nullString...)
		return nil
	}

	// 获取指针指向的值
	elemVal := src.Elem()

	// 使用预先缓存的元素编码器
	elemEncoder := getEncoder(e.elemType)
	return elemEncoder.appendToBytes(stream, elemVal)
}
