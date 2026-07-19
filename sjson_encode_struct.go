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
	keyBytes  []byte // 预计算的键字节："name":
	index     []int
	offset    uintptr // 预计算的 unsafe 偏移量（支持嵌套匿名字段累加）
	omitempty bool
	typ       reflect.Type
	encoder   Encoder // 预缓存字段编码器
}

// fieldByIndex 根据索引路径获取字段值，支持匿名字段的多级路径
//
//go:inline
func fieldByIndex(v reflect.Value, index []int) reflect.Value {
	if len(index) == 1 {
		return v.Field(index[0])
	}
	return v.FieldByIndex(index)
}

// fieldByUnsafeOffset 通过预计算的偏移量直接访问字段值，绕过 reflect.Value.Field() 的运行时检查
// 这是 OPT-1 的核心：用 unsafe.Pointer(base + offset) + reflect.NewAt() 构造 reflect.Value
// 相比 fieldByIndex 可减少每字段 5+ 次内部类型/可设性检查
// 当值不可寻址时回退到 fieldByIndex 保证安全性
//
//go:inline
func fieldByUnsafeOffset(v reflect.Value, offset uintptr, index []int, typ reflect.Type) reflect.Value {
	if v.CanAddr() {
		base := unsafe.Pointer(v.UnsafeAddr())
		return reflect.NewAt(typ, unsafe.Pointer(uintptr(base)+offset)).Elem()
	}
	return fieldByIndex(v, index)
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
	f := fieldByUnsafeOffset(src, field.offset, field.index, field.typ)

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
		f := fieldByUnsafeOffset(src, field.offset, field.index, field.typ)
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
		f := fieldByUnsafeOffset(src, field.offset, field.index, field.typ)

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
