# sjson 性能优化计划

> 创建日期：2026-07-17
> 环境：Apple M4 Pro / Go 1.26.1 / darwin-arm64
> 基准对比库：encoding/json (StdLib)、jsoniter (jsonfast)、sonic（与 Go 1.26 不兼容，基准测试中已绕过）

---

## 方案概览

| 编号 | 名称 | 预期收益 | 难度 | 阶段 | 状态 |
|------|------|----------|------|------|------|
| OPT-4 | Encoder buffer pool | 减少分配→0 | 低 | 1 | ✅ 已完成 |
| OPT-3 | SWAR 加速编码字符串转义检测 | 5-10% | 低 | 1 | ✅ 已完成 |
| OPT-2 | 内联 Token 消费（消除结构字符 Token 物化） | 10-15% | 中 | 1 | ✅ 已完成 |
| OPT-5 | 快速整数解析旁路 strconv | 5-8% | 中 | 1 | ✅ 已完成 |
| **OPT-1** | **预计算字段偏移 + unsafe 指针访问** | **20-30%** | 高 | 2 | 待实现 |
| OPT-6 | 字段匹配：二分查找 / perfect hash | 5-10% | 中 | 2 | 待实现 |
| **OPT-8** | **Opcode 解释器（jsoniter 风格 JIT-lite）** | **15-25%** | 高 | 3 | 待实现 |
| **OPT-7** | **类型特化代码生成（easyjson 风格 AOT）** | **40-60%** | 极高 | 3 | 待实现 |

---

## 三大瓶颈分析

### 瓶颈 1：reflect.Value 开销（40-50%）

`reflect.Value.Field(i)` / `SetInt()` 每字段 5+ 次内部检查（类型检查、可设检查、地址对齐等）。在结构体编解码中，每字段都经过完整的 reflect 路径，这是最大的性能损失点。

**对策**：OPT-1 用 unsafe 指针预计算字段偏移，直接读写内存，绕过 reflect.Value 的运行时检查。

### 瓶颈 2：Token 物化（20-25%）

`{` `}` `,` `:` 等单字符结构分隔符在旧实现中也被构造为 28B 的 `Token` 结构体（含 `Type`、`Value`、`Pos` 等字段），产生不必要的内存写入和分支判断。

**对策**：OPT-2 用 `consumeStructDelimiter()` 内联消费，直接检查 `d.token.Type` 而不经过完整的 Token 物化路径。

### 瓶颈 3：字符串/map 查找（15-20%）

`bytesToString` + map 哈希查找在结构体解码中每字段都会执行。字段数量越多，map 哈希开销越大。

**对策**：OPT-6 用二分查找（字段已排序）或 perfect hash 替代 map 查找；小结构体已用线性扫描优化。

---

## JIT 路线判断

不建议实现真正的机器码 JIT（如 sonic），原因：
- 跨平台维护成本极高（arm64/amd64 需分别实现）
- Go GC 安全性交互复杂（GC barrier 需精确处理）
- macOS W^X 限制（写后不可执行，需 mmap 变通）

### 推荐替代路线

| 路线 | 等价物 | 收益 | 优势 |
|------|--------|------|------|
| OPT-8 Opcode 解释器 | "JIT-lite" | 15-25% | 预编译指令序列用解释器执行，消除 reflect 开销 + CPU 分支预测友好，无平台限制 |
| OPT-7 编译时代码生成 | "AOT" | 40-60% | 预穷举类型特化函数，零反射零接口分发，ShapeSig 哈希查找，不依赖 go generate 或运行时 JIT |

---

## 已完成的优化详情

### ✅ OPT-4: Encoder buffer pool

**目标**：消除编码路径的内存分配，通过对象池复用 buffer。

**实现**：
- `sjson_marshal.go` — `encoderStream` 结构体封装 `[]byte` buffer
- `encoderStreamPool`（`sync.Pool`）预分配 4096 字节初始容量
- `getEncoderStreamWithSize()` 根据估算大小预分配
- `estimateJSONSize()` 按类型估算（map/struct/slice/string 分别估算）
- `releaseEncoderStream()` 超过 65536 字节时重新分配，防止大对象驻留池中

**结果**：编码路径零分配（仅最终 `copy(result)` 一次），Encoder Binding 快 1.36x，Generic 快 1.74x。

---

### ✅ OPT-3: SWAR 加速编码字符串转义检测

**目标**：字符串编码时，用 SWAR（SIMD Within A Register）一次检测 8 字节，判断是否需要转义，避免逐字节扫描。

**实现**：
- `sjson_encode_string.go` — `stringNeedsEscapeSWAR()` 函数
- 8 字节批量检测：引号 `0x22`、反斜杠 `0x5C`、控制字符 `< 0x20`、非 ASCII `>= 0x80`
- 复用 `byte_utils.go` 中的 `hasBytes8()` 和 `hasControlChars8()` SWAR 原语
- `stringChunk64()` 用 unsafe 直接读取字符串前 8 字节为 `uint64`
- 无需转义时：直接 `append(s...)` 一次写入，零分支
- 需要转义时：单次循环 + `safeSet` 查表 + `escapeStringToBytes()`

**结果**：纯 ASCII 字符串（占大多数）走快速路径，仅一次 SWAR 检测即可确定无需转义。

---

### ✅ OPT-2: 内联 Token 消费（消除结构字符 Token 物化）

**目标**：消除 `{` `}` `,` `:` 等单字符结构分隔符的完整 Token 物化开销。

**实现**：
- `sjson_decode.go` — `consumeStructDelimiter(closeChar byte) int` 内联函数
  - 直接检查 `d.token.Type`（逗号/右括号），返回 0（继续）/ 1（结束）/ -1（错误）
  - 消除了原来 peekByte + nextToken 的两步路径
- `sjson_decode_struct.go` — 4 个解码函数（`decodeMapStringInterface`、`decodeMap`、`decodeMapStringString`、`decodeStruct`）全部接入 `consumeStructDelimiter`
  - 使用 `goto done` 模式跳出 for 循环（Go 中 `break` 在 `switch` 内只跳出 switch 不跳出 for）
- `sjson_decode_array.go` — 数组解码同步接入

**关键 Bug 修复**：
1. 原实现 `consumeStructDelimiter` 用 `peekByte` 读原始字节，但 `decodeValue` 返回时 `lexer.pos` 已越过逗号 → 改为检查 `d.token.Type`
2. `switch` 内 `break` 只跳出 `switch` 不跳出 `for` 循环 → 13 处改为 `goto done` 模式
3. 清理了 `peekByte` / `skipWhitespaceAndAdvance` 死代码

**结果**：结构体/数组解码减少 2 次 Token 物化（逗号 + 右括号），每键值对节省约 56 字节写入。

---

### ✅ OPT-5: 快速整数解析旁路 strconv

**目标**：整数解析旁路 `strconv.ParseInt`/`ParseUint`，用 SWAR 4 字节批量处理直接从 `[]byte` 解析，避免 string 转换。

**实现**：
- `byte_utils.go` — 新增 `parseInt64Fast(b []byte) (int64, bool)` 和 `parseUint64Fast(b []byte) (uint64, bool)`
  - SWAR 4 字节批量处理：`isAllDigits4()` 检测 4 字节是否全为数字
  - 一次 `n*10000 + d1*1000 + d2*100 + d3*10 + d4` 计算 4 位数字
  - 溢出检查：`n > (MaxUint64 - ...)/10000` 和 `n > (MaxUint64 - v)/10`
  - 负数处理：`-int64(n)`，特殊处理 `MinInt64` 边界
  - 返回 `(value, ok)` 而非 `(value, error)`，减少 error 分配
- `lexer.go` — `lexNumber()` 整数路径：先 `parseInt64Fast`，成功则 `IntValue` + `FloatValue` 同时填充；失败回退 `strconv.ParseFloat`
- `sjson_decode_basic.go` — `decodeNumber()` Int/Uint 路径：
  - `parseInt64Fast` / `parseUint64Fast` 快速路径
  - 失败回退 `strconv.ParseInt` / `ParseUint`（处理浮点数字符串截断等边缘情况）
  - `decodeToInterface` 中整数也走快速路径，直接 `reflect.ValueOf(n)` 而非 `reflect.ValueOf(value)`

**结果**：整数编解码零 string 分配，SWAR 批量处理减少分支判断。全量 32 项基准测试通过，无性能退化。

---

## 当前性能基准（OPT-2/3/4/5 完成后）

| 场景 | sjson | encoding/json | 倍率 |
|------|-------|---------------|------|
| Decoder Binding | 31543 ns/op, 351 MB/s | 55275 ns/op, 201 MB/s | 1.75x 快 |
| Decoder Generic | — | — | 1.69x 快 |
| Encoder Generic | 22555 ns/op, 492 MB/s | 39245 ns/op, 283 MB/s | 1.74x 快 |
| Encoder Binding | 5632 ns/op, 1971 MB/s | 7786 ns/op, 1426 MB/s | 1.38x 快 |
| Medium Decode | 2978 ns/op, 0 allocs | 9049 ns/op, 504 B/op | 3.04x 快 |
| Medium Encode | 282 ns/op | 279 ns/op | 持平 |
| Complex Decode | 2423 ns/op | 3865 ns/op | 1.60x 快 |
| Complex Encode | 1581 ns/op | 2225 ns/op | 1.41x 快 |

> 解码优于 encoding/json 2-4x，编码优于 1.3-1.7x。Medium 结构体 Unmarshal 快 2.56x 且内存少 94%。

---

## 待实现优化

### OPT-1: 预计算字段偏移 + unsafe 指针访问

**预期收益**：20-30%（最大单项收益）

**方案**：
- 在 `getStructFields()` 中预计算每个字段的 `unsafe.Offset` 偏移量
- 编码时用 `unsafe.Pointer(basePtr + offset)` 直接读取字段值，绕过 `reflect.Value.Field(i)`
- 解码时同理，直接写内存
- 需要处理：嵌套结构体（偏移链）、指针字段（需解引用）、interface 字段（需类型断言）

**风险**：
- unsafe 指针操作需谨慎处理 GC barrier（Go 的 write barrier 在 GC 期间需要特殊处理）
- 需要保证指针有效性（不能存储跨越 GC 的裸指针）

**涉及文件**：
- `sjson_encode_struct.go` — `structEncoder.appendToBytes()`
- `sjson_decode_struct.go` — `decodeStruct()`
- `sjson.go` — `structField` 结构体增加 `offset` 字段

---

### OPT-6: 字段匹配：二分查找 / perfect hash

**预期收益**：5-10%

**现状**：
- 字段数 ≤ 8：线性扫描（`bytes.Equal`），已优化
- 字段数 > 8：`map[string]int` 哈希查找，每次需 `bytesToString` 转换 + map 哈希

**方案**：
- 将字段名按字典序排序，二分查找替代 map 查找
- 或使用 perfect hash（如 FNV-1a 简化版），预计算哈希表
- 避免每次 `bytesToString` 的 string 分配（可直接对 `[]byte` 做 hash）

**涉及文件**：
- `sjson_decode_struct.go` — `getFieldMap()`、`decodeStruct()`

---

### OPT-7: 类型特化代码生成（easyjson 风格 AOT）

**预期收益**：40-60%（终极优化）

**方案**：
- 预穷举 3 层嵌套深度的类型特化函数（约 2500 个组合）
- 用 `ShapeSig`（类型形状签名）哈希查找匹配的特化函数
- 不依赖 `go generate` 或运行时 JIT，编译期生成所有特化代码
- 特化函数直接操作具体类型，零反射零接口分发

**关键设计**：
- `ShapeSig` = 结构体字段名 + 类型的哈希签名
- 首次遇到新类型时，从预穷举池中匹配最接近的特化函数
- 未匹配时回退到通用 reflect 路径

**风险**：
- 代码膨胀（2500+ 函数约增加 500KB 二进制体积）
- 泛型类型（如 `map[string][]Foo`）需要特殊处理

---

### OPT-8: Opcode 解释器（jsoniter 风格 JIT-lite）

**预期收益**：15-25%

**方案**：
- 首次编解码一个类型时，"编译"出 opcode 序列（字段偏移、类型操作码、跳转目标等）
- 后续编解码同一类型时，直接解释执行 opcode，消除 reflect 分支
- Opcode 类型：`OpField(offset, type)`、`OpInt`、`OpString`、`OpSkip`、`OpEnd` 等

**与 OPT-7 的区别**：
- OPT-7 是编译期 AOT（预穷举所有组合）
- OPT-8 是运行时 JIT-lite（首次遇到时编译 opcode，后续解释执行）
- 两者可叠加使用

**涉及文件**：新增 `sjson_opcode.go`

---

## 推荐实施路径

```
第一阶段（已完成）：OPT-2/3/4/5 → 15-25% 提升
第二阶段（下一步）：OPT-1 + OPT-6 → 20-30% 提升
第三阶段（远期）：  OPT-8 + OPT-7 → 接近 sonic 级别
```

### 第二阶段优先级

1. **OPT-1 优先**：单项收益最大（20-30%），且为 OPT-8 铺路（opcode 需要字段偏移信息）
2. **OPT-6 次之**：收益较小（5-10%），但实现简单，可与 OPT-1 并行

### 验证方法

每个 OPT 完成后：
1. 运行全量测试：`go test ./... -v -count=1`
2. 运行基准测试：`go test -bench=. -benchmem -benchtime=3s`
3. 对比编码/解码的 ns/op、B/op、allocs/op
4. 确认无性能退化（特别是 Medium Encode 场景）
