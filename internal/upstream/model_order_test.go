package upstream

import "testing"

// TestNaturalLess 钉住自然序比较器的核心语义：
// 数字段按数值（9 < 10，纯字典序会反过来）、前缀短的在前、其余按字节。
func TestNaturalLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"glm-5.1", "glm-5.2", true},
		{"glm-5.9", "glm-5.10", true},  // 数字段按数值：9 < 10
		{"glm-5.10", "glm-5.9", false}, // 反向
		{"hy3", "hy3-x", true},         // 前缀短的在前
		{"hy3-x", "hy3", false},
		{"deepseek-v4-flash", "deepseek-v4-pro", true},
		{"deepseek-v4-pro", "deepseek-v4.1-flash", true}, // '-' < '.' 字节序
		{"glm-5.3", "glm-5v-turbo", true},                // '.' < 'v'
		{"a", "a", false},                                // 相等不 less
		{"", "a", true},                                  // 空串最短
		{"auto", "deepseek-v4-flash", true},
	}
	for _, c := range cases {
		if got := naturalLess(c.a, c.b); got != c.want {
			t.Errorf("naturalLess(%q,%q)=%v want %v", c.a, c.b, got, c.want)
		}
		if c.a != c.b && naturalLess(c.b, c.a) == c.want {
			t.Errorf("antisymmetry violated for (%q,%q)", c.a, c.b)
		}
	}
}

// TestSortModelInfosByName 钉住出口排序：与实际目录同形的一组 id（含 5.9/5.10
// 数值序对照）排成既定期望顺序，且重排幂等。
func TestSortModelInfosByName(t *testing.T) {
	ids := []string{
		"space-bunny", "glm-5.2", "auto", "hy4-preview-f", "glm-5.10", "glm-5.9",
		"deepseek-v4.1-flash", "glm-5.3-flash", "hy3-x", "deepseek-v4-pro",
		"glm-5v-turbo", "glm-5.3", "glm-5.1", "hy3", "kimi-k2.8-preview",
		"deepseek-v4-flash", "kimi-k2.6", "kimi-k2.7", "minimax-m3", "hy4-preview", "kimi-k3-1",
	}
	infos := make([]ModelInfo, 0, len(ids))
	for _, id := range ids {
		infos = append(infos, ModelInfo{ID: id})
	}
	sortModelInfosByName(infos)
	got := make([]string, 0, len(infos))
	for _, mi := range infos {
		got = append(got, mi.ID)
	}
	want := []string{
		"auto",
		"deepseek-v4-flash", "deepseek-v4-pro", "deepseek-v4.1-flash",
		"glm-5.1", "glm-5.2", "glm-5.3", "glm-5.3-flash", "glm-5.9", "glm-5.10", "glm-5v-turbo",
		"hy3", "hy3-x", "hy4-preview", "hy4-preview-f",
		"kimi-k2.6", "kimi-k2.7", "kimi-k2.8-preview", "kimi-k3-1",
		"minimax-m3", "space-bunny",
	}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sorted[%d]=%q want %q\n got: %v\nwant: %v", i, got[i], want[i], got, want)
		}
	}
	// 幂等：重排后再次排序不再变化。
	sortModelInfosByName(infos)
	for i := range want {
		if infos[i].ID != want[i] {
			t.Fatalf("re-sort changed order at %d: %q", i, infos[i].ID)
		}
	}
}

// TestSortNamesByName 名字列表同比较器（global 出口用）。
func TestSortNamesByName(t *testing.T) {
	names := []string{"gpt-5.10", "gpt-5.9", "gpt-6-astra", "deep-model"}
	sortNamesByName(names)
	want := []string{"deep-model", "gpt-5.9", "gpt-5.10", "gpt-6-astra"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names=%v want %v", names, want)
		}
	}
}
