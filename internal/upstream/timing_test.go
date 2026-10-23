package upstream

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/misakano7545/workbuddy2api-panel/internal/auth"
)

// ChatTiming 只做观测：nil 出参不得影响行为；非 nil 时 prep/do 都要有测量值。
//
// 计时精度说明：本机（Windows）的 time.Now() 只到毫秒级，fake transport 秒回的调用
// 会被记成 0（实测 -count=5 里 4 次 Prep/Do 全 0，总耗时也是 0）。故这里给 fake
// transport 加 20ms 延迟保证 Do 可测，并用哨兵初值验证 Prep 的 += 累加确实执行——
// 不依赖平台计时精度，也不放松断言。
func TestChatTimingObservation(t *testing.T) {
	const netDelay = 20 * time.Millisecond
	// 哨兵：Prep/Do 都是 += 累加，计时代码没跑到时值仍是哨兵。
	gotTiming := ChatTiming{Prep: time.Nanosecond, Do: time.Nanosecond}
	c := testClient(func(r *http.Request) (*http.Response, error) {
		time.Sleep(netDelay)
		return &http.Response{StatusCode: 200,
			Header: http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:   io.NopCloser(strings.NewReader("data: [DONE]\n\n"))}, nil
	})
	a := &auth.Auth{AccessToken: "at", UID: "u1"}
	// 大 body：prepareBody 的 JSON 解析耗时才能被毫秒级计时器测到（37 字节的小 body
	// 在 Windows 上恒记 0，prep 段永远测不出来）。
	body := []byte(`{"model":"glm-5.2","messages":[{"role":"user","content":"` +
		strings.Repeat("x", 1<<20) + `"}]}`)

	rc, _, _, err := c.ChatStreamContext(nil, a, body, "", ChatMeta{CacheKeyID: "s"}, &gotTiming)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	rc.Close()
	if gotTiming.Prep <= time.Nanosecond {
		t.Errorf("Prep 未累加（哨兵未变）：%+v", gotTiming)
	}
	if gotTiming.Do < netDelay {
		t.Errorf("Do 未测量（应含 fake transport 的 %v 延迟）：%+v", netDelay, gotTiming)
	}
	if gotTiming.Total() != gotTiming.Prep+gotTiming.Do {
		t.Errorf("Total 应等于两段之和：%+v", gotTiming)
	}

	// nil 出参：零开销路径，不得 panic 也不得改变返回。
	rc2, status, _, err := c.ChatStreamContext(nil, a, body, "", ChatMeta{}, nil)
	if err != nil || status != 200 {
		t.Fatalf("nil timing 路径：status=%d err=%v", status, err)
	}
	rc2.Close()

	// 零值语义：未测量时 Total 为 0（不是负值/NaN）。
	var zero ChatTiming
	if zero.Total() != 0 || zero.Prep != 0 {
		t.Errorf("零值语义被破坏：%+v", zero)
	}
}
