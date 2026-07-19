package sjson

import (
	"fmt"
	"io"
	"reflect"
	"sync"
)

// 用于缓存反射值的对象池
var valueSlicePool = sync.Pool{
	New: func() interface{} {
		// 预分配8个元素的容量，这是一个平衡点
		s := make([]reflect.Value, 0, 8)
		return &s
	},
}

// 用于缓存接口数组的对象池 - 增大初始容量
var interfaceSlicePool = sync.Pool{
	New: func() interface{} {
		s := make([]interface{}, 0, 16)
		return &s
	},
}

// 解码器对象池
var decoderPool = sync.Pool{
	New: func() interface{} {
		// 创建一个新的解码器，包含一个新的Lexer
		return &Decoder{
			lexer: NewLexer(nil),
		}
	},
}

// Decoder 直接从JSON文本解码到Go对象，无需中间Value对象
type Decoder struct {
	lexer  *Lexer
	token  Token
	config Config
}

// 重置解码器状态
func (d *Decoder) reset(input []byte, config Config) {
	d.lexer.Reset(input)
	d.config = config
	d.token = Token{}
}

// 创建新的直接解码器
func newDecoder(input []byte, config Config) *Decoder {
	d := decoderPool.Get().(*Decoder)
	d.reset(input, config)
	d.nextToken() // 读取第一个token
	return d
}

// 释放解码器回对象池
func releaseDecoder(d *Decoder) {
	d.lexer.input = nil // 避免持有大对象的引用
	decoderPool.Put(d)
}

// 从io.Reader创建新的直接解码器
func newDecoderFromReader(r io.Reader, config Config) (*Decoder, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return newDecoder(data, config), nil
}

// 读取下一个token - 内联优化
//
//go:inline
func (d *Decoder) nextToken() {
	d.token = d.lexer.NextToken()
}

// consumeStructDelimiter 检查并消费结构分隔符（, 或 } 或 ]）
// 返回: 0 = 逗号（继续）, 1 = 右括号（结束）, -1 = 错误
// 此时 d.token 已经是逗号或右括号（由 decodeValue 的 nextToken 读取）
//
//go:inline
func (d *Decoder) consumeStructDelimiter(closeChar byte) int {
	if d.token.Type == CommaToken {
		d.nextToken()
		return 0 // 继续
	}
	// closeChar '}' → RightBraceToken, ']' → RightBracketToken
	expectedType := RightBraceToken
	if closeChar == ']' {
		expectedType = RightBracketToken
	}
	if d.token.Type == expectedType {
		d.nextToken()
		return 1 // 结束
	}
	return -1 // 错误
}

// 直接解码到目标对象
func (d *Decoder) Decode(v interface{}) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("解码目标必须是非nil指针")
	}

	// 解码值到指针所指向的对象
	if err := d.decodeValue(rv); err != nil {
		return err
	}

	// 确保解析完毕，没有多余的token
	if d.token.Type != EOFToken {
		return fmt.Errorf("JSON解析完成后存在多余内容: %v", d.token)
	}

	return nil
}
