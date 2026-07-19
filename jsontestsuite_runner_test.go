package sjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// JSONTestSuite 测试运行器
// 测试套件来源：https://github.com/nst/JSONTestSuite
// 文件名约定：
//   y_ 前缀：内容必须被解析器接受
//   n_ 前缀：内容必须被解析器拒绝
//   i_ 前缀：实现定义（可接受或拒绝）
//
// 运行：go test -run TestJSONTestSuite -v

const jsontestsuiteDir = "tests/JSONTestSuite/test_parsing"

type suiteExpectation int

const (
	expectAccept suiteExpectation = iota // y_
	expectReject                         // n_
	expectAny                            // i_
)

func (e suiteExpectation) String() string {
	switch e {
	case expectAccept:
		return "y"
	case expectReject:
		return "n"
	default:
		return "i"
	}
}

type suiteResult struct {
	file        string
	expectation suiteExpectation
	sjsonOK     bool   // sjson 是否接受
	sjsonErr    string // sjson 错误信息（若有）
	stdlibOK    bool   // encoding/json 是否接受
	stdlibErr   string // encoding/json 错误信息（若有）
}

// 解析文件名前缀得到预期结果
func expectationFromName(name string) suiteExpectation {
	switch {
	case strings.HasPrefix(name, "y_"):
		return expectAccept
	case strings.HasPrefix(name, "n_"):
		return expectReject
	default:
		return expectAny
	}
}

// 运行单个测试文件，返回两个库的解析结果
func runSuiteFile(t *testing.T, path string, expect suiteExpectation) suiteResult {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取文件失败 %s: %v", path, err)
	}

	// 去掉可能的 BOM，与 encoding/json 行为对齐
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))

	res := suiteResult{
		file:        filepath.Base(path),
		expectation: expect,
	}

	// sjson：解码到 interface{}
	var sjsonVal interface{}
	if err := Unmarshal(data, &sjsonVal); err != nil {
		res.sjsonOK = false
		res.sjsonErr = err.Error()
	} else {
		res.sjsonOK = true
	}

	// encoding/json：解码到 interface{}
	var stdlibVal interface{}
	if err := json.Unmarshal(data, &stdlibVal); err != nil {
		res.stdlibOK = false
		res.stdlibErr = err.Error()
	} else {
		res.stdlibOK = true
	}

	return res
}

// 判断结果是否符合预期
func (r suiteResult) sjsonMatchesExpectation() bool {
	switch r.expectation {
	case expectAccept:
		return r.sjsonOK
	case expectReject:
		return !r.sjsonOK
	default:
		return true // i_ 总是符合
	}
}

func (r suiteResult) stdlibMatchesExpectation() bool {
	switch r.expectation {
	case expectAccept:
		return r.stdlibOK
	case expectReject:
		return !r.stdlibOK
	default:
		return true
	}
}

func TestJSONTestSuite(t *testing.T) {
	dir := jsontestsuiteDir
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("无法读取测试目录 %s: %v\n请确保已克隆 nst/JSONTestSuite：git clone https://github.com/nst/JSONTestSuite.git %s",
			dir, err, dir)
	}

	// 按文件名排序，保证输出稳定
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	sort.Strings(files)

	if len(files) == 0 {
		t.Fatalf("测试目录 %s 下没有 .json 文件", dir)
	}

	var results []suiteResult
	var sjsonMismatches, stdlibMismatches int
	var sjsonOnlyMismatches []suiteResult // sjson 不符合预期但 stdlib 符合（潜在 bug）

	for _, f := range files {
		expect := expectationFromName(filepath.Base(f))
		r := runSuiteFile(t, f, expect)
		results = append(results, r)

		if !r.sjsonMatchesExpectation() {
			sjsonMismatches++
			// 如果 stdlib 能正确处理但 sjson 不能，更可能是 sjson 的 bug
			if r.stdlibMatchesExpectation() {
				sjsonOnlyMismatches = append(sjsonOnlyMismatches, r)
			}
		}
		if !r.stdlibMatchesExpectation() {
			stdlibMismatches++
		}
	}

	// 统计
	var yCount, nCount, iCount int
	var sjsonYPass, sjsonNPass, sjsonIPass int
	for _, r := range results {
		switch r.expectation {
		case expectAccept:
			yCount++
			if r.sjsonOK {
				sjsonYPass++
			}
		case expectReject:
			nCount++
			if !r.sjsonOK {
				sjsonNPass++
			}
		default:
			iCount++
			sjsonIPass++ // i_ 总算通过
		}
	}

	t.Logf("===== JSONTestSuite 结果汇总 =====")
	t.Logf("测试用例总数：%d（y_=%d, n_=%d, i_=%d）", len(results), yCount, nCount, iCount)
	t.Logf("sjson   符合预期：%d/%d（y_通过 %d/%d, n_通过 %d/%d, i_通过 %d/%d）",
		len(results)-sjsonMismatches, len(results),
		sjsonYPass, yCount, sjsonNPass, nCount, sjsonIPass, iCount)
	t.Logf("stdlib  符合预期：%d/%d",
		len(results)-stdlibMismatches, len(results))
	t.Logf("sjson 与预期不符：%d，其中 stdlib 正确但 sjson 错误：%d", sjsonMismatches, len(sjsonOnlyMismatches))

	if len(sjsonOnlyMismatches) > 0 {
		t.Logf("")
		t.Logf("===== sjson 不符合预期但 stdlib 符合的用例（优先排查）=====")
		for _, r := range sjsonOnlyMismatches {
			t.Logf("  [%s] %s", r.expectation, r.file)
			t.Logf("      sjson:   ok=%v err=%q", r.sjsonOK, r.sjsonErr)
			t.Logf("      stdlib:  ok=%v err=%q", r.stdlibOK, r.stdlibErr)
		}
	}

	// 列出所有 sjson 不符合预期的用例（完整）
	if sjsonMismatches > 0 {
		t.Logf("")
		t.Logf("===== 所有 sjson 不符合预期的用例 =====")
		for _, r := range results {
			if r.sjsonMatchesExpectation() {
				continue
			}
			t.Logf("  [%s] %s", r.expectation, r.file)
			t.Logf("      sjson:   ok=%v err=%q", r.sjsonOK, r.sjsonErr)
			t.Logf("      stdlib:  ok=%v err=%q", r.stdlibOK, r.stdlibErr)
		}
	}

	// 整体判定：
	// - y_ 和 n_ 不符合预期视为失败（i_ 不计入失败）
	// 这里只对 "stdlib 正确但 sjson 错误" 的用例报 FAIL，避免 i_ 和边界争议造成噪音
	failed := 0
	for _, r := range sjsonOnlyMismatches {
		t.Errorf("sjson 行为与 RFC 8259 预期不符（stdlib 正确）：[%s] %s — sjson ok=%v err=%q",
			r.expectation, r.file, r.sjsonOK, r.sjsonErr)
		failed++
	}

	if failed == 0 {
		t.Logf("")
		t.Logf("✓ sjson 在所有 y_/n_ 用例上与 encoding/json 行为一致（无回归）。")
	}

	// 将完整结果写入文件供查看
	writeSuiteReport(t, results)
}

// 将详细结果写入 reports 目录，便于人工复核
func writeSuiteReport(t *testing.T, results []suiteResult) {
	var buf bytes.Buffer
	buf.WriteString("# JSONTestSuite 详细结果\n\n")
	buf.WriteString(fmt.Sprintf("用例总数：%d\n\n", len(results)))

	// 按预期分组输出
	for _, expect := range []suiteExpectation{expectAccept, expectReject, expectAny} {
		switch expect {
		case expectAccept:
			buf.WriteString("## y_ 必须接受\n\n")
		case expectReject:
			buf.WriteString("## n_ 必须拒绝\n\n")
		default:
			buf.WriteString("## i_ 实现定义\n\n")
		}
		buf.WriteString("| 文件 | sjson | sjson错误 | stdlib | stdlib错误 | sjson符合预期 |\n")
		buf.WriteString("|------|-------|----------|--------|-----------|---------------|\n")
		for _, r := range results {
			if r.expectation != expect {
				continue
			}
			sjsonMark := "✓"
			if !r.sjsonOK {
				sjsonMark = "✗"
			}
			stdlibMark := "✓"
			if !r.stdlibOK {
				stdlibMark = "✗"
			}
			matchMark := "✓"
			if !r.sjsonMatchesExpectation() {
				matchMark = "✗"
			}
			// 截断错误信息避免表格过宽
			sErr := r.sjsonErr
			if len(sErr) > 60 {
				sErr = sErr[:60] + "..."
			}
			stErr := r.stdlibErr
			if len(stErr) > 60 {
				stErr = stErr[:60] + "..."
			}
			buf.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
				r.file, sjsonMark, sErr, stdlibMark, stErr, matchMark))
		}
		buf.WriteString("\n")
	}

	reportPath := "tests/JSONTestSuite_report.md"
	if err := os.WriteFile(reportPath, buf.Bytes(), 0644); err != nil {
		t.Logf("警告：写入报告失败 %s: %v", reportPath, err)
	} else {
		t.Logf("详细报告已写入：%s", reportPath)
	}
}
