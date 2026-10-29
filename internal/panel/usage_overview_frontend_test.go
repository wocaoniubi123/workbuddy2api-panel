package panel

// 用量总览 KPI 的前端切片测试（仿 credit_frontend_adversarial_test.go 的 node + vm 做法）。
// 钉住「平均缓存命中率」卡：位置在「请求数」「总 token」之后、口径 = 命中/(命中+未命中)、
// 无样本时显示占位符；字符串脏值不得抛异常。只新增测试，不改业务代码。

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const usOverviewScript = `const fs = require('fs');
const vm = require('vm');
const src = fs.readFileSync(process.argv[2], 'utf8');
const escStart = src.indexOf('function esc(');
const escEnd = src.indexOf('function ago(');
const cacheStart = src.indexOf('function cacheRateText');
const cacheEnd = src.indexOf('function fmtBytes');
const kpiStart = src.indexOf('function fmtTok(n)');
const kpiEnd = src.indexOf('/* renderUsageChart');
if ([escStart, escEnd, cacheStart, cacheEnd, kpiStart, kpiEnd].some(v => v < 0))
  throw new Error('usage overview helpers not found');
const els = {};
const $ = id => (els[id] || (els[id] =
  { innerHTML: '', textContent: '', title: '', dataset: {}, addEventListener() {}, onchange: null }));
const ctx = { Date, Number, String, Math, RegExp, isNaN, parseFloat, parseInt, JSON, $ };
vm.createContext(ctx);
// 切片外的依赖以桩代之：模块级状态、时区/备注/图表与两个缓存格式化函数。
vm.runInContext(
  'let usDim="account", usCreditDim="account", usSort="total", usageData=null;' +
  'function trangeLabel(){ return "近7天"; }' +
  'function remarkName(u, h){ return h; }' +
  'function renderUsageChart(){ }' +
  // renderUsage 末尾还会填按小时表，那个函数在切片之外（切片止于 renderUsageChart 注释）。
  'function renderUsageHours(){ }' +
  'function fmtHit(){ return "—"; }' +
  'function cacheRateCell(){ return "—"; }' +
  src.slice(escStart, escEnd) + src.slice(cacheStart, cacheEnd) + src.slice(kpiStart, kpiEnd) +
  '\nthis.renderUsage=renderUsage;', ctx);
const out = {};
const run = (key, fn) => { try { fn(); out[key] = true; } catch (e) { out[key] = String((e && e.stack) || e); } };

run('sampleOk', () => ctx.renderUsage({ totals: {
  requests: 10, errors: 1, prompt_tokens: 9000, completion_tokens: 1000, total_tokens: 10000,
  avg_latency_ms: 1200, avg_tokens_per_second: 42.5,
  credits: 1.23, credit_samples: 3,
  cache_hit_tokens: 7200, cache_miss_tokens: 800 }, buckets: 3 }));
out.sampleStats = els.usStats.innerHTML;

run('emptyOk', () => ctx.renderUsage({ totals: {} }));
out.emptyStats = els.usStats.innerHTML;

run('dirtyOk', () => ctx.renderUsage({ totals: { requests: '3', cache_hit_tokens: '7200', cache_miss_tokens: 'x' } }));
out.dirtyStats = els.usStats.innerHTML;
console.log(JSON.stringify(out));
`

// TestUsageOverviewCacheHitKPI 验证总览第三张主指标卡「平均缓存命中率」的渲染。
func TestUsageOverviewCacheHitKPI(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; usage overview frontend test skipped")
	}
	f, err := os.CreateTemp(t.TempDir(), "us-kpi-*.cjs")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(usOverviewScript); err != nil {
		t.Fatal(err)
	}
	f.Close()
	raw, err := exec.Command(node, f.Name(), "app.js").CombinedOutput()
	if err != nil {
		t.Fatalf("usage overview node test failed: %v\n%s", err, raw)
	}
	var out struct {
		SampleOk    bool   `json:"sampleOk"`
		SampleStats string `json:"sampleStats"`
		EmptyOk     bool   `json:"emptyOk"`
		EmptyStats  string `json:"emptyStats"`
		DirtyOk     bool   `json:"dirtyOk"`
		DirtyStats  string `json:"dirtyStats"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("解析 node 输出失败: %v\n%s", err, raw)
	}
	for _, c := range []struct {
		ok  bool
		msg string
	}{
		{out.SampleOk, "正常样本渲染抛异常"},
		{out.EmptyOk, "空 totals 渲染抛异常"},
		{out.DirtyOk, "字符串脏值渲染抛异常"},
	} {
		if !c.ok {
			t.Fatalf("renderUsage %s", c.msg)
		}
	}

	stats := out.SampleStats
	if !strings.Contains(stats, "平均缓存命中率") {
		t.Fatalf("总览缺少「平均缓存命中率」卡:\n%s", stats)
	}
	if !strings.Contains(stats, "90%") {
		t.Fatalf("命中率 7200/8000 应显示 90%%:\n%s", stats)
	}
	// 本仓 fmtTok 是亿/万制（万以下原样、不加 k 缩写），7200 显示 "7200" 而非上游的 "7.2k"。
	if !strings.Contains(stats, "命中 7200 · 未命中 800") {
		t.Fatalf("副标题应回显命中/未命中 token:\n%s", stats)
	}
	// 积分与样本数：上游本轮新增「总积分」独立卡，但本仓把它并入「消耗积分」卡的副标题
	// （两者取的是同一字段 t.credits，见 app.js renderUsage 注释），故这里断言副标题。
	if !strings.Contains(stats, "消耗积分") || !strings.Contains(stats, "3 条积分样本") {
		t.Fatalf("消耗积分卡应回显 usage.credit 样本数:\n%s", stats)
	}
	// 位置：请求数 → 总 token → 平均缓存命中率 → 输入（主指标同排；本仓标签是中文输入/输出）。
	iReq := strings.Index(stats, ">请求数<")
	iTotal := strings.Index(stats, ">总 token<")
	iHit := strings.Index(stats, ">平均缓存命中率<")
	iPrompt := strings.Index(stats, ">输入<")
	if !(iReq >= 0 && iReq < iTotal && iTotal < iHit && iHit < iPrompt) {
		t.Fatalf("卡片顺序应为 请求数 → 总 token → 平均缓存命中率 → 输入:\n%s", stats)
	}

	// 无样本时不伪造 0：本仓把上游的「总积分」并进「消耗积分」卡，副标题口径写「无积分样本」。
	if !strings.Contains(out.EmptyStats, "无积分样本") {
		t.Fatalf("无积分样本时应提示「无积分样本」而不是伪造 0:\n%s", out.EmptyStats)
	}
	if !strings.Contains(out.EmptyStats, "无缓存观测样本") || !strings.Contains(out.EmptyStats, ">—<") {
		t.Fatalf("无样本时应显示占位 — 与「无缓存观测样本」:\n%s", out.EmptyStats)
	}
	// 脏值：'7200'/'x' 应按 Number() 归零/取值，不产生 NaN%。
	if strings.Contains(out.DirtyStats, "NaN") {
		t.Fatalf("脏值渲染出 NaN:\n%s", out.DirtyStats)
	}
}

// TestUsageOverviewK8Grid 钉住 index.html：usStats 用 k8 栅格（一行放下全部总览卡）。
// 上游是 8 张卡（k8 = 八列）；本 fork 额外有「消耗积分」「使用状态」两张、并把上游的
// 「总积分」并入消耗积分卡，实际共 9 张，故栅格为九列，类名沿用上游的 k8 不变。
func TestUsageOverviewK8Grid(t *testing.T) {
	raw, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)
	if !strings.Contains(html, `class="kpis k8" id="usStats"`) {
		t.Fatal("usStats 容器应使用 kpis k8 栅格")
	}
	if !strings.Contains(html, ".kpis.k8 { grid-template-columns: repeat(9") {
		t.Fatal("缺少 .kpis.k8 九列栅格规则（fork 总览共 9 张卡）")
	}
	if !strings.Contains(html, "@media (max-width: 700px) { .kpis, .kpis.k4, .kpis.k8 {") {
		t.Fatal("窄屏断点应把 k8 一并折成 2 列")
	}
}
