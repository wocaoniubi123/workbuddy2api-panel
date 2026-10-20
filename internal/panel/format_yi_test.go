package panel

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAppJSFmtTokYi 钉住 fmtTok 的亿制分支：token 单位按「亿」显示，不退回 M/K/B 缩写。
//
// 背景：面板所有 token 列走 fmtTok（≥1e8 收两位小数用「亿」，其余千分位）；请求日志行
// 与窄表格走另一套 fmtTokShort（k/M/B）。两条口径刻意分开，改动一边不得串到另一边。
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
  belowYi: f(99999999),
  thousands: f(12345),
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
	// 期望值直接由实现推得：1e8→"1.00亿"；1.23456789e8→"1.23亿"；99999999 未过亿→千分位；
	// 负数/非数字不进亿分支（Number('abc')=NaN，String(Math.round(NaN))="NaN"）。
	want := `{"exactlyYi":"1.00亿","fracYi":"1.23亿","belowYi":"99,999,999",` +
		`"thousands":"12,345","zero":"0","negative":"-1,000","notNumber":"NaN","nilValue":"0"}`
	if strings.TrimSpace(string(out)) != want {
		t.Fatalf("fmtTok 输出不符:\n got=%s\nwant=%s", strings.TrimSpace(string(out)), want)
	}
	// 反向：fmtTokShort 必须仍走 k/M/B（防止有人把两条口径合并回一条）。
	raw, err := os.ReadFile(filepath.Join("app.js"))
	if err != nil {
		t.Fatal(err)
	}
	js := string(raw)
	if !strings.Contains(js, "function fmtTokShort(") {
		t.Fatal("fmtTokShort 不见了：请求日志/图表刻度的 k/M/B 口径被合并进 fmtTok？")
	}
	if !strings.Contains(js, "(n / 1e9).toFixed(2) + 'B'") {
		t.Error("fmtTokShort 的 B（十亿）档不见了")
	}
}
