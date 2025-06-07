package sjson

import (
	"reflect"
)

// structField 表示结构体字段的缓存信息
type structField struct {
	name      []byte
	index     int
	omitempty bool
	typ       reflect.Type
	encoder   Encoder // 预缓存字段编码器
}

type structEncoder struct {
	typ          reflect.Type
	fields       []structField
	numFields    int  // 字段数量，用于优化分发
	hasOmitEmpty bool // 是否有omitempty字段
}

// 添加appendToBytes方法，将结构体直接编码到字节切片
func (e *structEncoder) appendToBytes(stream *encoderStream, src reflect.Value) error {
	if src.Kind() == reflect.Ptr {
		if src.IsNil() {
			stream.buffer = append(stream.buffer, nullString...)
			return nil
		}
		src = src.Elem()
	}

	// 预估缓冲区大小，减少重新分配
	estimatedSize := e.estimateSize()
	if cap(stream.buffer)-len(stream.buffer) < estimatedSize {
		newBuf := make([]byte, len(stream.buffer), len(stream.buffer)+estimatedSize)
		copy(newBuf, stream.buffer)
		stream.buffer = newBuf
	}

	// 开始对象
	stream.buffer = append(stream.buffer, '{')

	// 根据字段数量选择不同的编码策略
	switch e.numFields {
	case 0:
		// 空结构体，直接返回
		stream.buffer = append(stream.buffer, '}')
		return nil
	case 1:
		// 单字段优化：直接处理，无需循环
		return e.encodeSingleField(stream, src)
	default:
		// 多字段：根据是否有omitempty选择策略
		if e.hasOmitEmpty {
			return e.encodeFieldsWithOmitEmpty(stream, src)
		} else {
			return e.encodeFieldsFast(stream, src)
		}
	}
}

// 估算编码后的大小
func (e *structEncoder) estimateSize() int {
	// 基础大小：{} + 字段名引号和冒号
	size := 2
	for _, field := range e.fields {
		// 字段名 + 引号 + 冒号 + 逗号 + 估算值大小
		size += len(field.name) + 4 + 20 // 20是值的估算大小
	}
	return size
}

// 单字段编码优化
func (e *structEncoder) encodeSingleField(stream *encoderStream, src reflect.Value) error {
	field := e.fields[0]
	f := src.Field(field.index)

	// 处理omitempty
	if field.omitempty && isEmptyValue(f) {
		stream.buffer = append(stream.buffer, '}')
		return nil
	}

	// 写入字段名
	stream.buffer = append(stream.buffer, '"')
	stream.buffer = append(stream.buffer, field.name...)
	stream.buffer = append(stream.buffer, '"', ':')

	// 编码字段值
	err := field.encoder.appendToBytes(stream, f)
	if err != nil {
		return err
	}

	stream.buffer = append(stream.buffer, '}')
	return nil
}

// 快速编码（无omitempty字段）
func (e *structEncoder) encodeFieldsFast(stream *encoderStream, src reflect.Value) error {
	for i, field := range e.fields {
		// 添加逗号分隔符
		if i > 0 {
			stream.buffer = append(stream.buffer, ',')
		}

		// 写入字段名
		stream.buffer = append(stream.buffer, '"')
		stream.buffer = append(stream.buffer, field.name...)
		stream.buffer = append(stream.buffer, '"', ':')

		// 编码字段值
		f := src.Field(field.index)
		err := field.encoder.appendToBytes(stream, f)
		if err != nil {
			return err
		}
	}

	stream.buffer = append(stream.buffer, '}')
	return nil
}

// 带omitempty的编码
func (e *structEncoder) encodeFieldsWithOmitEmpty(stream *encoderStream, src reflect.Value) error {
	firstField := true

	for _, field := range e.fields {
		f := src.Field(field.index)

		// 处理omitempty标签
		if field.omitempty && isEmptyValue(f) {
			continue
		}

		// 添加逗号分隔符
		if !firstField {
			stream.buffer = append(stream.buffer, ',')
		}
		firstField = false

		// 写入字段名
		stream.buffer = append(stream.buffer, '"')
		stream.buffer = append(stream.buffer, field.name...)
		stream.buffer = append(stream.buffer, '"', ':')

		// 编码字段值
		err := field.encoder.appendToBytes(stream, f)
		if err != nil {
			return err
		}
	}

	stream.buffer = append(stream.buffer, '}')
	return nil
}
