package sjson

import (
	"bytes"
	"reflect"
	"sync"
)

// OPT-6: 排序后的字段名数组 + 下标映射，用于二分查找替代 map 查找
// 避免每次 bytesToString 的 string 分配，直接对 []byte 做二分比较
var sortedFieldsCache = sync.Map{} // map[reflect.Type]*sortedFields

// sortedFields 预排序的字段查找结构
type sortedFields struct {
	names   [][]byte // 按字典序排序的字段名
	indices []int    // 对应 fields 切片中的下标
}

// getSortedFields 获取预排序的字段查找结构（带缓存）
func getSortedFields(structType reflect.Type, fields []structField) *sortedFields {
	if cached, ok := sortedFieldsCache.Load(structType); ok {
		return cached.(*sortedFields)
	}

	sf := &sortedFields{
		names:   make([][]byte, len(fields)),
		indices: make([]int, len(fields)),
	}
	for i, field := range fields {
		sf.names[i] = field.name
		sf.indices[i] = i
	}
	// 插入排序：字段数通常不多，且仅在首次构建时执行一次
	for i := 1; i < len(sf.names); i++ {
		for j := i; j > 0 && bytes.Compare(sf.names[j], sf.names[j-1]) < 0; j-- {
			sf.names[j], sf.names[j-1] = sf.names[j-1], sf.names[j]
			sf.indices[j], sf.indices[j-1] = sf.indices[j-1], sf.indices[j]
		}
	}

	sortedFieldsCache.Store(structType, sf)
	return sf
}

// searchFieldBinary 在排序的字段名数组中二分查找 keyBytes，返回 fields 下标（未找到返回 -1）
//
//go:inline
func searchFieldBinary(sf *sortedFields, keyBytes []byte) int {
	lo, hi := 0, len(sf.names) - 1
	for lo <= hi {
		mid := lo + (hi-lo)/2
		cmp := bytes.Compare(keyBytes, sf.names[mid])
		if cmp == 0 {
			return sf.indices[mid]
		}
		if cmp < 0 {
			hi = mid - 1
		} else {
			lo = mid + 1
		}
	}
	return -1
}
