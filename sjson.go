package sjson

import (
	"reflect"
	"strings"
	"sync"
)

// 编解码器缓存部分
var (
	// 结构体字段信息缓存
	structFieldsCache sync.Map // map[reflect.Type][]structField
)

// Config 用于配置JSON解析和编码的行为
type Config struct {
	// SortMapKeys 控制对象和map的键是否排序，默认不排序
	SortMapKeys bool
}

// 默认配置
var defaultConfig = Config{
	SortMapKeys: false,
}

// SetDefaultConfig 设置默认的全局配置
func SetDefaultConfig(config Config) {
	defaultConfig = config
}

// GetDefaultConfig 获取当前默认配置
func GetDefaultConfig() Config {
	return defaultConfig
}

// rawFieldInfo 收集字段时的中间结构，记录字段深度以便解决同名冲突
// （规则参考 encoding/json：深度越浅优先级越高；同一深度出现同名字段则都丢弃）
type rawFieldInfo struct {
	name      string
	index     []int
	omitempty bool
	asString  bool // 是否指定了 json:",string" 选项（数字/布尔以字符串形式编解码）
	typ       reflect.Type
	depth     int
	tagged    bool // 是否显式通过 json tag 指定了名字（用于与匿名字段自身名字冲突时的优先级）
}

// collectRawFields 递归收集结构体字段，支持匿名（embedded）字段的提升
// 仅支持值类型的匿名结构体字段提升（不支持匿名指针字段，以保持实现简单可靠）
func collectRawFields(t reflect.Type, indexPrefix []int, depth int, out []rawFieldInfo) []rawFieldInfo {
	if depth > 16 {
		// 防止异常深度的嵌套（正常场景不会出现）
		return out
	}

	numField := t.NumField()
	for i := 0; i < numField; i++ {
		f := t.Field(i)

		// 跳过未导出字段（反射不可写；encoding/json 也会忽略）
		if f.PkgPath != "" && !f.Anonymous {
			continue
		}

		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}

		name := f.Name
		omitempty := false
		tagged := false

		tagName, options, _ := strings.Cut(tag, ",")
		if tagName != "" {
			name = tagName
			tagged = true
		}
		if options != "" {
			for options != "" {
				var opt string
				opt, options, _ = strings.Cut(options, ",")
				if opt == "omitempty" {
					omitempty = true
				}
			}
		}

		curIndex := make([]int, len(indexPrefix)+1)
		copy(curIndex, indexPrefix)
		curIndex[len(indexPrefix)] = i

		// 匿名字段：如果未显式指定 json tag 名字，且是结构体类型，则递归提升其字段
		if f.Anonymous && !tagged {
			ft := f.Type
			if ft.Kind() == reflect.Struct {
				out = collectRawFields(ft, curIndex, depth+1, out)
				continue
			}
			// 匿名的非结构体类型（如匿名 int、匿名接口等）按其类型名作为字段名处理，走下面通用逻辑
		}

		if f.PkgPath != "" {
			// 匿名字段递归失败或非结构体的未导出匿名类型，跳过
			continue
		}

		out = append(out, rawFieldInfo{
			name:      name,
			index:     curIndex,
			omitempty: omitempty,
			typ:       f.Type,
			depth:     depth,
			tagged:    tagged,
		})
	}

	return out
}

// resolveFieldConflicts 按 encoding/json 规则解决同名字段冲突：
// 深度越浅优先级越高；同一深度出现多个同名字段则全部丢弃
func resolveFieldConflicts(raw []rawFieldInfo) []rawFieldInfo {
	byName := make(map[string][]rawFieldInfo, len(raw))
	order := make([]string, 0, len(raw))
	for _, f := range raw {
		if _, ok := byName[f.name]; !ok {
			order = append(order, f.name)
		}
		byName[f.name] = append(byName[f.name], f)
	}

	result := make([]rawFieldInfo, 0, len(raw))
	for _, name := range order {
		candidates := byName[name]
		if len(candidates) == 1 {
			result = append(result, candidates[0])
			continue
		}

		// 找到最小深度
		minDepth := candidates[0].depth
		for _, c := range candidates[1:] {
			if c.depth < minDepth {
				minDepth = c.depth
			}
		}

		// 统计最小深度下有多少个候选
		var winners []rawFieldInfo
		for _, c := range candidates {
			if c.depth == minDepth {
				winners = append(winners, c)
			}
		}

		if len(winners) == 1 {
			result = append(result, winners[0])
		}
		// 多个同深度冲突：按 encoding/json 规则全部丢弃
	}

	return result
}

// 获取结构体类型的字段信息（支持匿名字段提升）
func getStructFields(t reflect.Type) []structField {
	if cachedFields, ok := structFieldsCache.Load(t); ok {
		return cachedFields.([]structField)
	}

	raw := collectRawFields(t, nil, 0, nil)
	resolved := resolveFieldConflicts(raw)

	fields := make([]structField, 0, len(resolved))
	for _, rf := range resolved {
		// 预缓存字段编码器
		fieldEncoder := getEncoder(rf.typ)

		// 预计算键字节
		keyBytes := make([]byte, 0, len(rf.name)+3)
		keyBytes = append(keyBytes, '"')
		keyBytes = append(keyBytes, rf.name...)
		keyBytes = append(keyBytes, '"', ':')

		// OPT-1: 预计算字段的 unsafe 偏移量
		// 对于多级索引路径（匿名字段提升），需逐级累加偏移量
		offset := uintptr(0)
		curType := t
		for _, idx := range rf.index {
			field := curType.Field(idx)
			offset += field.Offset
			curType = field.Type
		}

		nameBytes := stringToBytes(rf.name)
		fields = append(fields, structField{
			name:      nameBytes,
			keyBytes:  keyBytes,
			index:     rf.index,
			offset:    offset,
			nameLen:   len(nameBytes),
			nameHead:  head8(nameBytes),
			omitempty: rf.omitempty,
			typ:       rf.typ,
			encoder:   fieldEncoder,
		})
	}

	structFieldsCache.Store(t, fields)
	return fields
}
