package sjson

import (
	"reflect"
	"unsafe"
)

// structField 表示结构体字段的缓存信息
// index 使用 []int 以支持匿名（embedded）字段的多级索引路径
// offset 为预计算的 unsafe 偏移量（OPT-1），用于绕过 reflect.Value.Field() 开销
type structField struct {
	name      []byte
	keyBytes  []byte // 预计算的键字节："name":"
	index     []int
	offset    uintptr // 预计算的 unsafe 偏移量（支持嵌套匿名字段累加）
	nameLen   int     // 字段名长度，配合 nameHead 做 (len, head) 快速等值比较
	nameHead  uint64  // 字段名前 8 字节的小端序 uint64（不足 8 字节补零）
	omitempty bool
	typ       reflect.Type
	encoder   Encoder // 预缓存字段编码器
}

// fieldByIndex 根据索引路径获取字段值，支持匿名字段的多级路径
//
// 注：曾用 unsafe.Pointer(base+offset) + reflect.NewAt 合成 reflect.Value（OPT-1），
// 但 reflect.NewAt 内部的 ptrTo 未命中缓存时会分配新 rtype，反而比 Field(i) 更慢
// 且引入额外分配。offset 仍保留在 structField 上，供 opcode 编码路径直接读写内存。
//
//go:inline
func fieldByIndex(v reflect.Value, index []int) reflect.Value {
	if len(index) == 1 {
		return v.Field(index[0])
	}
	return v.FieldByIndex(index)
}

type structEncoder struct {
	typ          reflect.Type
	fields       []structField
	numFields    int                  // 字段数量，用于优化分发
	hasOmitEmpty bool                 // 是否有omitempty字段
	opcodes      *structOpcodeProgram // OPT-8: 标量结构体的预编译执行程序
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

	// 开始对象
	stream.buffer = append(stream.buffer, '{')

	// OPT-8/OPT-7: ShapeSig 匹配的无 omitempty 标量结构体走 opcode 快速路径。
	// 不可寻址值和复杂字段继续使用下方通用路径，保证语义一致。
	if e.opcodes != nil && e.opcodes.valid && !e.hasOmitEmpty && src.CanAddr() {
		return e.opcodes.appendToBytes(stream, unsafe.Pointer(src.UnsafeAddr()), e.fields)
	}

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

// 单字段编码优化
func (e *structEncoder) encodeSingleField(stream *encoderStream, src reflect.Value) error {
	field := e.fields[0]
	f := fieldByIndex(src, field.index)

	// 处理omitempty
	if field.omitempty && isEmptyValue(f) {
		stream.buffer = append(stream.buffer, '}')
		return nil
	}

	// 写入字段名
	stream.buffer = append(stream.buffer, field.keyBytes...)

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
		stream.buffer = append(stream.buffer, field.keyBytes...)

		// 编码字段值
		f := fieldByIndex(src, field.index)
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
		f := fieldByIndex(src, field.index)

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
		stream.buffer = append(stream.buffer, field.keyBytes...)

		// 编码字段值
		err := field.encoder.appendToBytes(stream, f)
		if err != nil {
			return err
		}
	}

	stream.buffer = append(stream.buffer, '}')
	return nil
}
