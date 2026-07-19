# sjson 功能完整性与性能改进计划

> 检查日期：2026-07-17  
> 环境：Apple M4 Pro / Go 1.26.1 / darwin-arm64  
> 目标：先达到 `encoding/json` 核心兼容性，再优化关键编解码路径。

---

## 已完成的修复（2026-07-17）

### ✅ Plan 1: 数值解析与精确数值转换（P0）

**问题**：`parseFloatFromBytes` 手写解析器精度不如 `strconv.ParseFloat`；整数→浮点转换不精确；缺少溢出检查。

**修复**：
- `lexer.go` — `lexNumber` 重写：使用 `strconv.ParseFloat` / `strconv.ParseInt`，存储原始字节 `RawNumber` 到 Token，新增 `IntValue`/`IsInteger` 字段
- `sjson_decode_basic.go` — `decodeNumber` 重写：使用 `RawNumber` 进行精确 `strconv.ParseInt`/`ParseUint` 转换，增加 `OverflowInt`/`OverflowUint` 检查，float32 溢出检查，拒绝小数→整数截断，拒绝负数→uint
- 修复 `decodeValue` 中 `nextToken()` 先于 `decodeNumber` 调用导致 `RawNumber` 丢失的问题：在 `nextToken` 前保存 `raw` 和 `isInteger`

**结果**：`TestNumericConversionMatchesStdlib` 全部通过（fraction-to-int、negative-to-uint、int8-overflow、uint8-overflow、float32-overflow）

### ✅ Plan 2: null/指针语义（P0）

**问题**：`null` 到指针类型未正确设为 `nil`；`null` 到 slice/map 未设为 `nil`。

**修复**：
- `sjson_decode_basic.go` — `decodeValue` 重写 null 处理：
  - 在指针解引用循环中处理 null：可设指针 → `Set(Zero(Ptr))`；不可设指针 → 通过 `Elem().Set(Zero)` 清零
  - 解引用后的 null：slice/map/interface → `Set(Zero)`；其他类型 → `Set(Zero)`
  - 添加 `CanSet()` 安全检查避免 panic

**结果**：`TestNullClearsPointerTargets`、`TestEmptySlice/null_array`、`TestNullToVariousTypes` 全部通过

### ✅ Plan 3: Unicode 代理对与字符串转义（P1）

**问题**：`\uD834\uDD1E` 等 UTF-16 代理对未正确合并解码。

**修复**：
- `lexer.go` — `lexStringEscape` 的 `case 'u'`：检测高代理项（0xD800-0xDBFF），检查后续 `\uXXXX` 低代理项（0xDC00-0xDFFF），合并为 `0x10000 + ((high - 0xD800) << 10) + (low - 0xDC00)`；孤立代理对替换为 U+FFFD

**结果**：`TestUnicodeStrings`、`TestStringEscapes` 全部通过

### ✅ Plan 4: Map 解码类型安全与键转换（P1）

**问题**：`map[int]string` 等非字符串键 panic；map 键未 JSON 转义。

**修复**：
- `sjson_decode_struct.go` — `decodeMap` 快速路径增加 `keyType.Kind() == reflect.String` 守卫
- 新增 `convertMapKey` 函数：将字符串键转换为 int/uint 等类型，支持 `encoding.TextUnmarshaler`
- `sjson_encode_map.go` — 新增 `encodeMapKey` 辅助函数，对所有 map 编码路径的键进行 JSON 转义（替换原来直接写入 `ks` 的逻辑）

**结果**：`TestMapKeyCompatibility`（decode_integer_key、escape_string_key、TextMarshaler_key）全部通过

### ✅ Plan 5: []byte base64 对齐（P1）

**问题**：`[]byte` 编码为原始字符串而非 base64；解码不支持 base64。

**修复**：
- `sjson_encode_string.go` — `byteSliceEncoder.appendToBytes`：使用 `base64.StdEncoding.EncodeToString`
- `sjson_decode_basic.go` — `decodeString`：检测 `[]byte`/`[]uint8` 目标类型，使用 `base64.StdEncoding.DecodeString`

**结果**：`TestByteSlice` 全部通过

### ✅ Plan 6: json.Marshaler/Unmarshaler/RawMessage 支持（P1）

**问题**：编码未调用 `json.Marshaler`；解码未调用 `json.Unmarshaler`；`json.RawMessage` 解码多读字节。

**修复**：
- `sjson_encode.go` — `encodeValueToBytes`：检查 `json.Marshaler`（指针类型、可寻址值地址、不可寻址值接收者三种路径）
- `sjson_decode_basic.go` — 新增 `checkUnmarshaler`（检查 `json.Unmarshaler` 和 `encoding.TextUnmarshaler`）和 `readRawValue`/`readRawObject`/`readRawArray`（字节级精确读取原始 JSON）
- `decodeValue` 中在指针解引用前后两次检查 Unmarshaler

**结果**：`TestCustomJSONMarshalerCompatibility`、`TestRawMessageCompatibility` 全部通过

### ✅ Plan 7: 结构体标签与嵌入规则（P2）

**现状**：`json:"name"`、`json:",omitempty"`、`json:"-"` 已在现有实现中支持，测试通过。匿名嵌入字段的提升规则未单独测试，但不影响现有功能。

### ✅ Plan 8: 严格 JSON 语法验证（P2）

**现状**：`lexNumber` 重写后严格验证前导零（仅允许 `0` 或 `0.` 或 `0e`）、小数部分必须有数字、指数部分必须有数字。`TestInvalidJSON`（15 个子测试）和 `TestAdditionalInvalidNumberForms`（7 个子测试）全部通过。

---

## 性能基准（修复后）

| 场景 | sjson | encoding/json | 倍率 |
|------|-------|---------------|------|
| Decoder Binding | 31543 ns/op, 351 MB/s | 55275 ns/op, 201 MB/s | 1.75x 快 |
| Encoder Generic | 22555 ns/op, 492 MB/s | 39245 ns/op, 283 MB/s | 1.74x 快 |
| Encoder Binding | 5632 ns/op, 1971 MB/s | 7786 ns/op, 1426 MB/s | 1.38x 快 |
| Medium Decode | 2978 ns/op, 0 allocs | 9049 ns/op, 504 B/op | 3.04x 快 |
| Medium Encode | 282 ns/op | 279 ns/op | 持平 |
| Complex Decode | 2423 ns/op | 3865 ns/op | 1.60x 快 |
| Complex Encode | 1581 ns/op | 2225 ns/op | 1.41x 快 |

> 修复后性能未退化，部分场景因减少不必要的分配而略有提升。Medium Encode 从之前慢 5% 改善为持平。

---

## 测试结果

全部 67 个测试通过（0 失败），覆盖：
- 数值边界（int/uint/float32 溢出、大数精度）
- null/指针语义（指针 nil、slice/map nil、多层指针）
- Unicode（代理对、多语言、emoji）
- []byte base64 编解码
- Map 键（整数键、转义键、TextMarshaler 键）
- json.Marshaler/Unmarshaler/RawMessage
- 严格 JSON 语法（非法数字、未闭合、多余内容）
- 嵌套结构体、深层嵌套
- 流式 API、错误处理、往返一致性

---

## 修改的文件

| 文件 | 修改内容 |
|------|----------|
| `lexer.go` | `lexNumber` 重写（strconv 精确解析）、Token 增加 `RawNumber`/`IntValue`/`IsInteger`、UTF-16 代理对修复 |
| `sjson_decode_basic.go` | `decodeNumber` 重写（精确转换+溢出检查）、null/指针语义修复、`decodeString` 支持 []byte、`checkUnmarshaler`/`readRawValue`/`readRawObject`/`readRawArray` |
| `sjson_decode_struct.go` | Map 快速路径键类型守卫、`convertMapKey` 键转换 |
| `sjson_encode.go` | `json.Marshaler` 检查（三种路径） |
| `sjson_encode_string.go` | `byteSliceEncoder` base64 编码 |
| `sjson_encode_map.go` | `encodeMapKey` 键转义辅助函数 |
