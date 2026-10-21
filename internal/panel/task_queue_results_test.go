package panel

import (
	"os"
	"strings"
	"testing"
	"time"
)

// TestPersistQueueResultsWritesTaskJobs 钉住「执行全部待办」的结果会写进 task-jobs.json。
//
// 背景：「刷新进度」读的就是这个文件，而队列结果此前只留在内存（q.items），
// 跑完刷新页面就没了、重启全丢——表现为"任务明明跑了，界面看不到"。
//
// 2026-10-19 实测复现：手动队列跑完 27 个账号的后台任务后，task-jobs.json 的 mtime
// 停在队列启动之前（19:54:51），于是「刷新进度」里只有走 taskjobs 的单账号结果。
func TestPersistQueueResultsWritesTaskJobs(t *testing.T) {
	const uid = "queue-results-uid"
	p, _, path := newTaskJobTestPanel(t, uid, "", nil)

	items := []queueItem{
		{UID: uid, Nickname: "号A", Kind: "growth", Code: "chat_5", Status: "done", Message: "已完成"},
		{UID: uid, Nickname: "号A", Kind: "growth", Code: "WorkBuddy_Valorant",
			Status: "done", Message: "已领奖", Credit: 100, Energy: 5},
		{UID: uid, Nickname: "号A", Kind: "account", Code: "", Status: "skipped", Message: "账号级占位"},
	}
	p.persistQueueResults(items, time.Now().Add(-time.Minute))

	m := p.getTaskJobManager()
	if m == nil {
		t.Fatal("taskjobs manager 未启动")
	}
	m.mu.Lock()
	job, ok := m.jobs[uid]
	m.mu.Unlock()
	if !ok {
		t.Fatal("job 没有写入内存")
	}
	if job.Trigger != "manual" || job.Status != "finished" {
		t.Fatalf("job 元信息不对: trigger=%q status=%q", job.Trigger, job.Status)
	}
	if job.Total != 2 {
		t.Fatalf("Total=%d，应为 2（账号级占位不计入）", job.Total)
	}
	if job.FinishedAt == nil {
		t.Error("FinishedAt 未设置")
	}

	codes := map[string]bool{}
	for _, r := range job.Results {
		if c, _ := r["task_code"].(string); c != "" {
			codes[c] = true
		}
	}
	for _, want := range []string{"chat_5", "WorkBuddy_Valorant"} {
		if !codes[want] {
			t.Errorf("结果里缺 %s（实际 %v）", want, codes)
		}
	}
	if len(codes) != 2 {
		t.Errorf("结果数=%d，应为 2（账号级占位不该写进去）: %v", len(codes), codes)
	}

	// 落盘检查：接口读的是文件，不是内存。
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "WorkBuddy_Valorant") {
		t.Error("task-jobs.json 里没有 WorkBuddy_Valorant，说明没落盘")
	}
}

// TestPersistQueueResultsKeepsUnlistedTaskCode 白名单外（补领）任务的结果也必须带
// task_code / desc。
//
// 为什么：上游 issue #174 让达标未领的白名单外任务也能入队（queuedActionFor 合成
// claimOnlyAction）。persistQueueResults 若仍用 autoActionFor，这些码拿到 nil，
// resultMap 的 `if act != nil` 会静默丢掉 task_code / desc——「刷新进度」只剩一条
// message，看不出是哪个任务补领的。走 queuedActionFor 后白名单内行为完全不变
// （它内部先调 autoActionFor）。
func TestPersistQueueResultsKeepsUnlistedTaskCode(t *testing.T) {
	const uid = "queue-unlisted-uid"
	const unlisted = "some_future_task_code"
	if autoActionFor(unlisted) != nil {
		t.Fatalf("%q 已在白名单里，换一个没登记的任务码", unlisted)
	}

	p, _, _ := newTaskJobTestPanel(t, uid, "", nil)
	p.persistQueueResults([]queueItem{
		{UID: uid, Nickname: "号B", Kind: "growth", Code: unlisted, Status: "done", Message: "补领成功", Credit: 50},
	}, time.Now().Add(-time.Minute))

	m := p.getTaskJobManager()
	if m == nil {
		t.Fatal("taskjobs manager 未启动")
	}
	m.mu.Lock()
	job, ok := m.jobs[uid]
	m.mu.Unlock()
	if !ok {
		t.Fatal("job 没有写入内存")
	}
	if len(job.Results) != 1 {
		t.Fatalf("Results 条数 = %d, want 1", len(job.Results))
	}
	r := job.Results[0]
	if code, _ := r["task_code"].(string); code != unlisted {
		t.Errorf("task_code = %q, want %q（白名单外应经 queuedActionFor 合成）", code, unlisted)
	}
	if desc, _ := r["desc"].(string); desc == "" {
		t.Error("desc 为空：白名单外任务缺少合成描述")
	}
	// 进度分子：countCompletedTaskResults 对 task_code 为空的条目直接跳过
	// （taskjobs.go:689-691）。修复前白名单外任务没有 task_code，补领的 done
	// 根本进不了 Completed——不只是「描述缺失」，进度条也少算了一格。
	if job.Completed != 1 {
		t.Errorf("Completed = %d, want 1（白名单外补领 status=done 必须计入进度）", job.Completed)
	}
}
