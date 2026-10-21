package panel

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAppJSFmtTokYi 钉住 fmtTok 的亿/万制：token 一律按「亿 / 万 / 原值」三级显示，
// 不再出现 M/K/B 缩写。
//
// 背景：面板所有 token 列（KPI、明细表、积分表、图表刻度、悬停卡片、请求日志行）
// 统一走这一套口径。历史上 fmtTokShort 另有过一套 k/M/B 缩写，已合并——同一屏里
// 两套单位最容易看错数量级。
func TestAppJSFmtTokYi(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; fmtTok test skipped")
	}
	script := `const fs = require('fs');
const vm = require('vm');
const src = fs.readFileSync(process.argv[2], 'utf8');
const start = src.indexOf('function fmtTok(');
const end = src.indexOf('/* fmtTokShort');
if (start < 0 || end < 0) throw new Error('fmtTok block not found');
const ctx = {};
vm.createContext(ctx);
vm.runInContext(src.slice(start, end) + '\nthis.fmtTok = fmtTok;', ctx);
const f = ctx.fmtTok;
process.stdout.write(JSON.stringify({
  exactlyYi: f(1e8),
  fracYi: f(123456789),
  wan: f(12345678),
  thousands: f(12345),
  small: f(9999),
  carryToYi: f(99999999),
  zero: f(0),
  negative: f(-1000),
  notNumber: f('abc'),
  nilValue: f(null),
}));`
	f, err := os.CreateTemp(t.TempDir(), "fmt-tok-*.cjs")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(script); err != nil {
		t.Fatal(err)
	}
	f.Close()
	out, err := exec.Command(node, f.Name(), "app.js").CombinedOutput()
	if err != nil {
		t.Fatalf("fmtTok node test failed: %v\n%s", err, out)
	}
	// 期望值由实现推得：亿档两位小数；万档两位小数；万以下原样（不加千分位）。
	// carryToYi：9999.9999 万四舍五入是 10000.00 万，必须进位成 1.00 亿。
	// 非数字/非有限值一律按 0（不再吐 "NaN"）。
	want := `{"exactlyYi":"1.00亿","fracYi":"1.23亿","wan":"1234.57万",` +
		`"thousands":"1.23万","small":"9999","carryToYi":"1.00亿","zero":"0",` +
		`"negative":"-1000","notNumber":"0","nilValue":"0"}`
	if strings.TrimSpace(string(out)) != want {
		t.Fatalf("fmtTok 输出不符:\n got=%s\nwant=%s", strings.TrimSpace(string(out)), want)
	}
	// 反向：fmtTokShort 必须与 fmtTok 同口径（曾被要求分开，现已统一）。
	raw, err := os.ReadFile(filepath.Join("app.js"))
	if err != nil {
		t.Fatal(err)
	}
	js := string(raw)
	if !strings.Contains(js, "function fmtTokShort(n) { return fmtTok(n); }") {
		t.Error("fmtTokShort 不再委托 fmtTok：两套单位口径又被拆开了？")
	}
	if strings.Contains(js, "(n / 1e9).toFixed(2) + 'B'") {
		t.Error("fmtTokShort 又出现了 B（十亿）缩写档，与统一口径冲突")
	}
}

// TestAppJSFmtNum 钉住 fmtNum 的口径：非 token 数字一律千分位，小数保留两位去尾零，
// 超亿用亿。
//
// 分工只有一条线：token 走 fmtTok（亿/万/原样），其余（计数、积分、额度、包面额）
// 全走 fmtNum。积分是 float64（后端 usage.go 的 Credits），小数位不能被四舍五入吃掉。
func TestAppJSFmtNum(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; fmtNum test skipped")
	}
	script := `const fs = require('fs');
const vm = require('vm');
const src = fs.readFileSync(process.argv[2], 'utf8');
// fmtNum 依赖 trimFixed（声明在它之后，靠函数提升），两段一起取。
const a = src.indexOf('function fmtNum(');
const b = src.indexOf('function fmtMs');
const c = src.indexOf('function trimFixed');
const d = src.indexOf('function fmtCreditRatio');
if ([a, b, c, d].some(v => v < 0)) throw new Error('fmtNum helpers not found');
const ctx = { Number, String, Math, RegExp };
vm.createContext(ctx);
vm.runInContext(src.slice(a, b) + src.slice(c, d) + '\nthis.fmtNum = fmtNum;', ctx);
const f = ctx.fmtNum;
process.stdout.write(JSON.stringify({
  integer: f(18500),
  big: f(220455),
  small: f(999),
  zero: f(0),
  decimal: f(0.12),
  decimal2: f(220455.99),
  round2: f(1234.567),
  yi: f(123456789),
  negative: f(-1000),
  negDecimal: f(-0.5),
  notNumber: f('abc'),
  nilValue: f(null),
}));`
	f, err := os.CreateTemp(t.TempDir(), "fmt-num-*.cjs")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(script); err != nil {
		t.Fatal(err)
	}
	f.Close()
	out, err := exec.Command(node, f.Name(), "app.js").CombinedOutput()
	if err != nil {
		t.Fatalf("fmtNum node test failed: %v\n%s", err, out)
	}
	// 小数位必须留住（积分是 float64）：0.12 → "0.12"，不能被舍成 "0"。
	// 非有限值统一 —（与 fmtTok 的 0 不同：那是「token 缺失=0」，这里是「没有这个数」）。
	want := `{"integer":"18,500","big":"220,455","small":"999","zero":"0",` +
		`"decimal":"0.12","decimal2":"220,455.99","round2":"1,234.57","yi":"1.23亿",` +
		`"negative":"-1,000","negDecimal":"-0.5","notNumber":"—","nilValue":"0"}`
	if strings.TrimSpace(string(out)) != want {
		t.Fatalf("fmtNum 输出不符:\n got=%s\nwant=%s", strings.TrimSpace(string(out)), want)
	}
}
