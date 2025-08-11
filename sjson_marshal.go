package sjson

import (
	"reflect"
	"sync"
)

// 编码器流对象池，用于减少内存分配
type encoderStream struct {
	buffer []byte
}

var encoderStreamPool = sync.Pool{
	New: func() interface{} {
		stream := &encoderStream{
			buffer: make([]byte, 0, 2048), // 增加初始容量
		}
		return stream
	},
}

// 获取一个编码器流
func getEncoderStream() *encoderStream {
	return encoderStreamPool.Get().(*encoderStream)
}

// 释放一个编码器流
func releaseEncoderStream(stream *encoderStream) {
	// 如果缓冲区过大，重新分配以避免内存泄漏
	if cap(stream.buffer) > 8192 {
		stream.buffer = make([]byte, 0, 2048)
	} else {
		stream.buffer = stream.buffer[:0]
	}
	encoderStreamPool.Put(stream)
}

// 估算JSON编码所需的缓冲区大小
func estimateJSONSize(v interface{}) int {
	switch val := v.(type) {
	case map[string]interface{}:
		return len(val) * 32 // 每个键值对估算32字节
	case []interface{}:
		return len(val) * 16 // 每个元素估算16字节
	case string:
		return len(val) + 16 // 字符串长度加上引号和转义字符
	case map[string]string:
		return len(val) * 24 // 字符串map较小
	case []string:
		return len(val) * 12 // 字符串数组
	default:
		return 256 // 默认大小
	}
}

// 获取带预估大小的编码器流
func getEncoderStreamWithSize(estimatedSize int) *encoderStream {
	stream := encoderStreamPool.Get().(*encoderStream)
	if cap(stream.buffer) < estimatedSize {
		stream.buffer = make([]byte, 0, estimatedSize)
	}
	return stream
}

// Marshal 使用直接编码模式将Go对象编码为JSON字节切片
func Marshal(v interface{}) ([]byte, error) {
	// 估算所需缓冲区大小并获取编码器流
	estimatedSize := estimateJSONSize(v)
	stream := getEncoderStreamWithSize(estimatedSize)
	defer releaseEncoderStream(stream)

	// 保存编码后的结果
	err := encodeValueToBytes(stream, reflect.ValueOf(v), reflect.TypeOf(v))
	if err != nil {
		return nil, err
	}

	result := append([]byte(nil), stream.buffer...)
	return result, nil
}

// MarshalString 使用直接编码模式将Go对象编码为JSON字符串
func MarshalString(v interface{}) (string, error) {
	// 复用 Marshal 函数并转换为字符串
	bytes, err := Marshal(v)
	if err != nil {
		return "", err
	}
	return bytesToString(bytes), nil
}
