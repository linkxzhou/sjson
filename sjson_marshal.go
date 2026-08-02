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
			buffer: make([]byte, 0, 4096), // 增加初始容量
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
	if cap(stream.buffer) > 65536 {
		stream.buffer = make([]byte, 0, 4096)
	} else {
		stream.buffer = stream.buffer[:0]
	}
	encoderStreamPool.Put(stream)
}

// 估算JSON编码所需的缓冲区大小
func estimateJSONSize(v interface{}) int {
	switch val := v.(type) {
	case map[string]interface{}:
		return len(val)*32 + 64 // 每个键值对估算32字节
	case []interface{}:
		return len(val)*16 + 32 // 每个元素估算16字节
	case string:
		return len(val) + 16 // 字符串长度加上引号和转义字符
	case map[string]string:
		return len(val)*24 + 32 // 字符串map较小
	case []string:
		return len(val)*12 + 16 // 字符串数组
	case []int:
		return len(val)*8 + 16
	default:
		// struct / 其他类型：用 reflect 估算字段数
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Ptr {
			rv = rv.Elem()
		}
		if rv.Kind() == reflect.Struct {
			return rv.NumField()*16 + 32
		}
		return 512
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

	// 保存编码后的结果
	err := encodeValueToBytes(stream, reflect.ValueOf(v), reflect.TypeOf(v))
	if err != nil {
		releaseEncoderStream(stream)
		return nil, err
	}

	// 复制结果（避免返回池中的缓冲区）
	result := make([]byte, len(stream.buffer))
	copy(result, stream.buffer)
	releaseEncoderStream(stream)

	return result, nil
}

// AppendMarshal 将 v 编码为 JSON 并追加到 dst 后面，返回追加后的切片。
// 允许调用方复用自己的缓冲区，避免 Marshal 每次固定的 make+copy 分配
// （对应 OPTIMIZATION_REVIEW.md §1.4 提到的"编码侧唯一剩下的分配来源"）。
func AppendMarshal(dst []byte, v interface{}) ([]byte, error) {
	estimatedSize := estimateJSONSize(v)
	stream := getEncoderStreamWithSize(estimatedSize)

	err := encodeValueToBytes(stream, reflect.ValueOf(v), reflect.TypeOf(v))
	if err != nil {
		releaseEncoderStream(stream)
		return dst, err
	}

	dst = append(dst, stream.buffer...)
	releaseEncoderStream(stream)
	return dst, nil
}

// MarshalString 使用直接编码模式将Go对象编码为JSON字符串
func MarshalString(v interface{}) (string, error) {
	// 估算所需缓冲区大小并获取编码器流
	estimatedSize := estimateJSONSize(v)
	stream := getEncoderStreamWithSize(estimatedSize)

	// 保存编码后的结果
	err := encodeValueToBytes(stream, reflect.ValueOf(v), reflect.TypeOf(v))
	if err != nil {
		releaseEncoderStream(stream)
		return "", err
	}

	// 直接转换为字符串（零拷贝，但需要复制以避免使用池中的缓冲区）
	result := string(stream.buffer)
	releaseEncoderStream(stream)

	return result, nil
}
