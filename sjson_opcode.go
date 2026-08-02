package sjson

import (
	"reflect"
	"sync"
	"unsafe"
)

// OPT-8: 结构体编码的运行时 opcode 程序。仅处理无 omitempty 的标量字段；
// 复杂字段保留原有 Encoder 回退，保证与 encoding/json 的兼容语义。
type structOpcode byte

const (
	opFallback structOpcode = iota
	opBool
	opInt
	opUint
	opFloat32
	opFloat64
	opString
)

type structOpcodeProgram struct {
	ops      []structOpcode
	shapeSig uint64
	valid    bool
}

// OPT-7: ShapeSig 缓存相同字段形状的 opcode 程序，避免重复分类。
var shapeProgramCache sync.Map // map[reflect.Type]*structOpcodeProgram

func newStructOpcodeProgram(t reflect.Type, fields []structField) *structOpcodeProgram {
	sig := shapeSignature(fields)
	if cached, ok := shapeProgramCache.Load(t); ok {
		return cached.(*structOpcodeProgram)
	}

	program := &structOpcodeProgram{
		ops:      make([]structOpcode, len(fields)),
		shapeSig: sig,
	}
	program.valid = true
	for i, field := range fields {
		program.ops[i] = opcodeForType(field.typ)
		if program.ops[i] == opFallback {
			program.valid = false
		}
	}
	actual, _ := shapeProgramCache.LoadOrStore(t, program)
	return actual.(*structOpcodeProgram)
}

func shapeSignature(fields []structField) uint64 {
	// FNV-1a；包含名称、类型和 omitempty，避免不同 JSON 语义共享程序。
	var h uint64 = 1469598103934665603
	for _, field := range fields {
		for _, c := range field.name {
			h ^= uint64(c)
			h *= 1099511628211
		}
		h ^= uint64(field.typ.Kind())
		h *= 1099511628211
		if field.omitempty {
			h ^= 1
			h *= 1099511628211
		}
	}
	return h
}

func opcodeForType(t reflect.Type) structOpcode {
	// 关键修复：具名类型（如 type Celsius float64）即使 Kind() 是标量，
	// 也可能实现了 json.Marshaler / encoding.TextMarshaler，必须调用其
	// 自定义方法而不能按裸类型直写内存，否则会产出与 encoding/json 不一致的结果。
	// （§1.7 指出的"ShapeSig 只能表达形状、无法表达真实类型"问题在此处的具体体现）
	if t.Implements(jsonMarshalerType) || t.Implements(textMarshalerType) {
		return opFallback
	}
	if t.Kind() != reflect.Ptr {
		pt := reflect.PointerTo(t)
		if pt.Implements(jsonMarshalerType) || pt.Implements(textMarshalerType) {
			return opFallback
		}
	}
	switch t.Kind() {
	case reflect.Bool:
		return opBool
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return opInt
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return opUint
	case reflect.Float32:
		return opFloat32
	case reflect.Float64:
		return opFloat64
	case reflect.String:
		return opString
	default:
		return opFallback
	}
}

// appendToBytes 执行预编译 opcode。调用方只对 valid 程序调用本函数。
func (p *structOpcodeProgram) appendToBytes(stream *encoderStream, base unsafe.Pointer, fields []structField) error {
	for i := range fields {
		if i > 0 {
			stream.buffer = append(stream.buffer, ',')
		}
		field := &fields[i]
		stream.buffer = append(stream.buffer, field.keyBytes...)
		ptr := unsafe.Add(base, field.offset)

		switch p.ops[i] {
		case opBool:
			if *(*bool)(ptr) {
				stream.buffer = append(stream.buffer, trueString...)
			} else {
				stream.buffer = append(stream.buffer, falseString...)
			}
		case opInt:
			stream.buffer = appendInt(stream.buffer, readInt(ptr, field.typ.Kind()), 10)
		case opUint:
			stream.buffer = appendUint(stream.buffer, readUint(ptr, field.typ.Kind()), 10)
		case opFloat32:
			if err := appendFloat32(stream, *(*float32)(ptr)); err != nil {
				return err
			}
		case opFloat64:
			if err := appendFloat64(stream, *(*float64)(ptr)); err != nil {
				return err
			}
		case opString:
			if err := encodeStringDirect(stream, *(*string)(ptr)); err != nil {
				return err
			}
		default:
			panic("sjson: invalid struct opcode program")
		}
	}
	stream.buffer = append(stream.buffer, '}')
	return nil
}

func readInt(ptr unsafe.Pointer, kind reflect.Kind) int64 {
	switch kind {
	case reflect.Int8:
		return int64(*(*int8)(ptr))
	case reflect.Int16:
		return int64(*(*int16)(ptr))
	case reflect.Int32:
		return int64(*(*int32)(ptr))
	case reflect.Int64:
		return *(*int64)(ptr)
	default:
		return int64(*(*int)(ptr))
	}
}

func readUint(ptr unsafe.Pointer, kind reflect.Kind) uint64 {
	switch kind {
	case reflect.Uint8:
		return uint64(*(*uint8)(ptr))
	case reflect.Uint16:
		return uint64(*(*uint16)(ptr))
	case reflect.Uint32:
		return uint64(*(*uint32)(ptr))
	case reflect.Uint64:
		return *(*uint64)(ptr)
	default:
		return uint64(*(*uint)(ptr))
	}
}
