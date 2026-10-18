package upstream

import (
	"sort"
	"strings"
)

// 模型列表输出顺序：名称自然序（数字段按数值）。
//
// 背景：CN 目录的 /v3/config 路（fetchV3Models）从 map 展开，遍历序每次随机，
// 面板「重新获取」与 /v1/models 的顺序每次都在跳；global 目录各探测路原始顺序
// 也不统一。两个域的出口（FetchModels / fetchGlobalModelsOnce）统一用本文件的
// 比较器排序——数字段按数值（glm-5.9 在 glm-5.10 前），同前缀短的在前
// （hy3 在 hy3-x 前），其余按字节（模型 id 全小写现状）。

// sortModelInfosByName 原地按模型 id 的自然序重排条目列表（nil/空为无操作）。
func sortModelInfosByName(infos []ModelInfo) {
	sort.Slice(infos, func(i, j int) bool { return naturalLess(infos[i].ID, infos[j].ID) })
}

// sortNamesByName 原地按模型名的自然序重排名字列表（nil/空为无操作）。
func sortNamesByName(names []string) {
	sort.Slice(names, func(i, j int) bool { return naturalLess(names[i], names[j]) })
}

// naturalLess 报告 a 是否应排在 b 之前（自然序，区别于纯字典序）：
//   - 两侧同为数字时按**数值**比较（去前导零后先比位数再比字典序；
//     数值相等但写法不同的按原文字节定序，保证全序确定）；
//   - 其余位置按字节比较（'-' < '.' < '0'-'9' < 'a'-'z'）；
//   - 一方是另一方前缀时短的在前（hy3 < hy3-x）；完全相等返回 false。
func naturalLess(a, b string) bool {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		ca, cb := a[i], b[j]
		if isASCIIDigit(ca) && isASCIIDigit(cb) {
			ia, ib := i, j
			for ia < len(a) && isASCIIDigit(a[ia]) {
				ia++
			}
			for ib < len(b) && isASCIIDigit(b[ib]) {
				ib++
			}
			ra := strings.TrimLeft(a[i:ia], "0")
			rb := strings.TrimLeft(b[j:ib], "0")
			switch {
			case len(ra) != len(rb):
				return len(ra) < len(rb)
			case ra != rb:
				return ra < rb
			case a[i:ia] != b[j:ib]:
				return a[i:ia] < b[j:ib]
			}
			i, j = ia, ib
			continue
		}
		if ca != cb {
			return ca < cb
		}
		i++
		j++
	}
	return i == len(a) && j != len(b)
}

// isASCIIDigit 报告 c 是否为 ASCII 数字。
func isASCIIDigit(c byte) bool { return c >= '0' && c <= '9' }
