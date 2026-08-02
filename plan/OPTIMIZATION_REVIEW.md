# sjson 优化方案复核与改进建议

> 复核日期：2026-08-01
> 复核对象：`OPTIMIZATION_PLAN.md`（OPT-1 ~ OPT-8）+ 当前代码实际实现
> 复核方式：逐文件阅读 `lexer.go` / `byte_utils.go` / `sjson_decode*.go` / `sjson_encode*.go` / `sjson_opcode.go` / `sjson_marshal.go`
> 说明：本文只做分析与设计，不含代码改动。收益数字均为**基于代码路径的估算**，需 profile 确认。

---

## 0. 一句话结论

现有计划的**方向判断（不做机器码 JIT、走 opcode + 偏移直写）是对的**，但：

1. `OPTIMIZATION_PLAN.md` 与代码**已经不同步**：OPT-1 / OPT-6 / OPT-8 标记为"待实现"，实际代码里都已落地（`fieldByUnsafeOffset`、`sjson_field_search.go`、`sjson_opcode.go`）。
2. 已落地的 OPT-1 / OPT-8 **大概率没有拿到预期收益，甚至是负优化**（下文 §1.1 / §1.4），`bench_report.html` 自己也记录了「0 allocs 退回 32B/2allocs」「Opcode 快速路径未触发」。
3. 计划**漏掉了三个成本最低、收益最确定的点**：lexer 空白扫描的负优化、Token 结构体 88 字节值拷贝、数字被解析两遍。
4. 剩余差距的真正大头是**解码侧仍然全程走 `reflect.Value`**（`decodeValue` → `SetInt/SetString/Set`），而不是字段偏移计算。
5. 有几个 SWAR 原语实现**是错的**（`isAllDigits4` 会把 `:` 判成数字、`hasControlChars8` 把所有非 ASCII 判成需慢路径），修正后既修 bug 又提速。

优先级建议：**先做 §2 的 A 组（零风险、成本低、立刻见效）→ 再做 B 组（结构性重构，收益最大）→ C 组按需**。
OPT-7（预穷举 2500 个特化函数）建议**直接放弃**（§4）。

---

## 1. 现有 OPT 方案逐项复核

### 1.1 OPT-1「预计算字段偏移 + unsafe 指针访问」——❗方向对，实现方式错

**计划说**：待实现，预期 20-30%。
**实际**：已实现。`sjson.go:186-194` 预计算 `offset`，`sjson_encode_struct.go:37-43` 的 `fieldByUnsafeOffset` 使用它。

问题在于实现方式：

```
fieldByUnsafeOffset: v.CanAddr() → v.UnsafeAddr() → reflect.NewAt(typ, base+offset).Elem()
```

- `reflect.NewAt` 内部需要 `typ.ptrTo()`。`PtrToThis` 命中时便宜，未命中时要走 `ptrMap`（`sync.Map`）查找**并可能分配一个新的 rtype**——这正好解释了 `bench_report.html` 里记录的 2 allocs 回退。
- `v.Field(i)` 本身在 Go runtime 里只是「读 `structType.fields[i]` + 组合 flag + 指针加 offset」，**并没有计划里说的"5+ 次内部检查"**。可设性检查是 `Set*` 时做的，不是 `Field()` 时做的。
- 所以 `CanAddr() + UnsafeAddr() + NewAt() + Elem()` 这条链**比 `Field(i)` 更长**，收益为负是完全可能的。

**结论**：`offset` 字段本身要保留（它是后续方案的基础），但**不要用它去合成 `reflect.Value`**。偏移量的价值只有在「彻底不再构造 `reflect.Value`、直接 `*(*int64)(ptr) = v`」时才能兑现（见 §2.B1 / §2.B2）。

**立即可做的验证**：把 `fieldByUnsafeOffset` 临时改回 `fieldByIndex` 跑 `BenchmarkCompareMedium`，如果 Unmarshal 从 32B/2allocs 回到 0B/0allocs 且不变慢，就证实了上述判断。

### 1.2 OPT-2「内联 Token 消费」——⚠️ 收益被高估，前提没成立

计划声称"消除结构字符 Token 物化，每键值对节省约 56 字节写入"。

实际上 `consumeStructDelimiter`（`sjson_decode.go:86`）只是**把 `if/else if` 换成了返回码 + `switch`**：

- `,` `}` `]` 依然由 `l.NextToken()` **完整构造 Token 并整体赋值给 `d.token`**（`d.token = d.lexer.NextToken()`）。
- Token 现在是 `Type(8) + FloatValue(8) + IntValue(8) + IsInteger(1+7pad) + Value(24) + RawNumber(24) + Pos(8) ≈ 88 字节`。**物化一次都没少**，只少了一两次分支。

也就是说：OPT-2 描述的收益（省 56B 写入）**并未真正发生**，真正要省它必须做 §2.A2。另外它带来的 13 处 `goto done` 让代码可读性下降，属于付了成本没拿到收益。

**结论**：保留现状（无害），但收益记账要改；真正的 Token 开销消除见 §2.A2。

### 1.3 OPT-3「SWAR 转义检测」——⚠️ 有效，但被错误的原语拖累

`stringNeedsEscapeSWAR` 逻辑本身合理，但它依赖的 `hasControlChars8`（`byte_utils.go:404`）实现是：

```
(chunk - 0x2020202020202020) & 0x8080808080808080 != 0
```

这个式子对**任何 ≥ 0x80 的字节都返回 true**。后果：

- 任何含中文/emoji/非 ASCII 的字符串（中文 JSON 极常见）**100% 退化到逐字节循环**，SWAR 完全失效；
- 借位还会污染相邻字节，产生额外误报。

正确的「存在字节 < 0x20」SWAR 判据是 `(chunk - 0x20···) & ^chunk & 0x8080···`。修正后非 ASCII 不再误报。

进一步：编码侧对非 ASCII 字节**根本不需要转义**（慢路径里 `utf8.DecodeRuneInString` 后原样透传，输出等价），所以 `stringNeedsEscapeSWAR` 完全可以把 ≥0x80 当成安全字节，**让中文字符串也走 `append(s...)` 一次性快路径**。

**结论**：OPT-3 值得保留并**修正原语**（§2.A4），修正后对非 ASCII 负载是数倍差距，不是 5-10%。

### 1.4 OPT-4「Encoder buffer pool」——✅ 有效，但描述与代码不符 + 有效性打折

- 计划说 `estimateJSONSize()` "按 map/struct/slice/string 分别估算"，**代码里没有 struct 分支**（`sjson_marshal.go:39`），struct 一律 512。
- `getEncoderStreamWithSize` 在 `cap < estimatedSize` 时**直接替换 `stream.buffer`**，被替换掉的旧 buffer 就丢了；由于池初值是 4096 而估算值最大 512，实际这条分支几乎不触发，等于 estimate 是空转。
- `Marshal` 结尾固定 `make + copy` 一次（不可避免，因为要返回独立切片），但**没有提供让调用方复用 buffer 的 API**，这是编码侧唯一剩下的分配来源。

**结论**：保留，但把"估算"换成"per-type 学习实际输出大小"，并补 `AppendMarshal`/`MarshalTo`（§2.C2）。

### 1.5 OPT-5「快速整数解析」——⚠️ 有效但存在两处硬问题

**(a) `isAllDigits4` 判据不正确**（`byte_utils.go:433`）：

```
adjusted := chunk - 0x30303030
mask := adjusted & 0xF0F0F0F0
underflow := (chunk ^ 0x30303030) & 0x80808080
return mask == 0 && underflow == 0
```

对 `:`（0x3A）：`0x3A-0x30 = 0x0A`，`0x0A & 0xF0 == 0`；`(0x3A^0x30)&0x80 == 0` → **被判定为数字**，随后 `d = ':' - '0' = 10` 参与运算，得到错误数值。`;` `<` `=` `>` `?` 同样误判。
目前靠 lexer 已先做过语法扫描、只把纯数字 `RawNumber` 喂进来才没爆出问题，属于**埋着的雷**。

无借位污染的正确判据（推荐）：

```
(chunk & 0xF0F0F0F0) == 0x30303030  &&  ((chunk + 0x06060606) & 0xF0F0F0F0) == 0x30303030
```

验证：`'9'`=0x39 → 0x30 ✓ / 0x3F&0xF0=0x30 ✓；`':'`=0x3A → 0x30 ✓ / 0x40&0xF0=0x40 ✗ 正确拒绝；`'/'`=0x2F → 0x20 ✗ 正确拒绝。

**(b) 整数被解析了两遍**。这是计划完全没意识到的浪费：

- `lexNumber`（`lexer.go:435`）已经 `parseInt64Fast(raw)` 得到 `IntValue`，并且**还额外算了 `FloatValue: float64(n)`**；
- `decodeNumber`（`sjson_decode_basic.go:620`）拿到 `raw` 后**再 `parseInt64Fast(raw)` 一次**。

浮点更糟：`lexNumber` 无条件 `strconv.ParseFloat`（最贵的操作之一），即使目标字段是 `float32`、`string`、或该字段根本不存在（会被 `skipValue` 丢弃）也照样付费。

**结论**：OPT-5 应升级为「延迟解析」（§2.A3），这是解码侧成本最低的一笔大收益。

### 1.6 OPT-6「二分查找字段匹配」——⚠️ 已实现，但选错了算法

已实现（`sjson_field_search.go`）。问题：

- 二分需要 `log2(n)` 次 `bytes.Compare`，每次都是**函数调用 + 逐字节比较**，对 n=16 是 4 次调用；
- 更关键：`getSortedFields` 每次 `decodeStruct` 都要做一次 `sync.Map.Load`（`decodeStruct` 里已经有 `getStructFields` 的一次 Load，加上这次 = 每个 struct 至少 2 次 `sync.Map.Load`）；
- 阈值 8 是硬编码的，8 个字段做 8 次 `bytes.Equal` 也不便宜。

**更优方案**：`(len, 前 8 字节当作 uint64)` 二元组比较（jsoniter / sonic 的做法）。JSON 字段名 ≤ 8 字节的占绝大多数，此时**一次 uint64 相等比较即可判定，零 memcmp、零函数调用**；>8 字节才追加一次 `bytes.Equal` 校验。详见 §2.A5。

### 1.7 OPT-7「预穷举 ~2500 个类型特化函数」——❌ 建议放弃

- `ShapeSig` 只能表达"形状"，无法表达真实类型：字段名相同、Kind 相同但类型不同（`type Celsius float64` vs `float64`、实现了 `json.Marshaler` 的命名类型）会**撞同一个特化函数并产出错误结果**；
- 2500 个函数不可能覆盖真实类型空间（嵌套 + 泛型容器组合是指数级），命中率极低；
- 500KB 二进制膨胀 + 巨大的测试面。

`sjson_opcode.go` 里目前的 `shapeSignature()` 就体现了这个设计的问题：**它算出了 `sig` 却
从未被读取比较（`shapeSig` 字段写入后无任何 `.shapeSig ==` 使用），只靠 `sync.Map` 按 `reflect.Type`
做缓存，`ShapeSig` 本身是死代码**。因为当前 `sjson_opcode.go` 只是一个「按 Kind 分派的标量 opcode
解释器」（覆盖 bool/int/uint/float/string 六种裸类型），并不是文档最初设想的"预穷举 2500 个特化函数"，
所以不需要额外删除代码，维持现状即可（`shapeSig` 字段可保留作为将来扩展位，不必现在清理）。

---

## 5. 修改状态清单（本轮全部完成，2026-08-02）

> 文档原文在此处（第 133 行）硬截断，§2 的 A/B/C 分组方案与 §3/§4 正文从未写完，
> 会话历史中也是同一份截断内容，无法恢复原始设计细节。以下清单基于 §1 逐项复核结论
> 及复核过程中发现的问题实施，逐项标注状态：

| 编号 | 内容 | 状态 | 说明 |
|------|------|------|------|
| §1.1 OPT-1 | `fieldByUnsafeOffset` 回退为 `fieldByIndex` | ✅ 已完成 | 消除 `reflect.NewAt` 潜在的 rtype 分配；`offset` 字段保留供 opcode 路径使用 |
| §1.2 OPT-2 | Token 结构体缩减 | ✅ 已完成 | 合并 `RawNumber` 到 `Value`，Token 由 ~88B 降至 ~64B |
| §1.3 OPT-3 | SWAR 转义检测原语修正 | ✅ 已完成 | `hasControlChars8` 改用无借位污染判据；非 ASCII 字节不再误报为需转义 |
| §1.4 OPT-4 | Encoder buffer pool 改进 | ✅ 已完成 | `estimateJSONSize` 补充 struct 分支（按字段数估算）；新增 `AppendMarshal` 支持调用方复用缓冲区 |
| §1.5(a) OPT-5 | `isAllDigits4` SWAR 判据修正 | ✅ 已完成 | 改用无借位污染公式，正确拒绝 `:` `;` `<` `=` `>` `?` |
| §1.5(b) OPT-5 | 数字延迟/避免重复解析 | ✅ 已完成 | `decodeNumber` 直接复用 lexer 已算好的 `IntValue`，不再对 `raw` 重复 `parseInt64Fast` |
| §1.6 OPT-6 | 字段匹配算法改进 | ✅ 已完成 | 由二分查找 + 排序缓存改为 `(len, head8 uint64)` 二元组比较（jsoniter/sonic 同款），删除 `sjson_field_search.go` |
| §1.7 OPT-7 | 预穷举特化函数 | ✅ 无需处理 | 代码里从未真正实现「2500 个特化函数」，`sjson_opcode.go` 只是按 Kind 分派的标量 opcode 解释器，维持现状 |
| A1（新增） | lexer 空白扫描负优化 | ✅ 已完成 | `isAllWhitespace8` 由逐字节循环改为 4 次 SWAR 零字节检测，无分支 |
| 新发现 #1 | opcode 快速路径绕过 `json.Marshaler`/`TextMarshaler` | ✅ 已修复 | 具名标量类型（如 `type Celsius float64` 实现 `MarshalJSON`）此前会被 opcode 直写内存，产出错误结果；已在 `opcodeForType` 增加接口检测，命中时回退 `opFallback` |
| 新发现 #2 | `getFieldMap`/`fieldMapCache` 死代码 | ✅ 已清理 | 精确匹配已改用 `(len, head8)`，原 map 版本精确匹配从未被调用，已删除 |

**遗留/未验证项**（因文档截断、无法确认原设计意图，未处理）：
- §2 B 组「解码侧彻底摆脱 `reflect.Value`，直接按 opcode 写内存」（`decodeValue → SetInt/SetString/Set` 仍是主要开销来源，§0 第 4 点提到但方案细节缺失）。
- §2 C 组「per-type 学习实际输出大小」替代当前的静态估算表（已按可读到的 §1.4 结论补充 struct 分支，更精细的学习式估算需要新方案）。

以上两项建议后续如有新的设计文档补充后再实施，避免在方案不完整的情况下做结构性重构。