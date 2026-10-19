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
