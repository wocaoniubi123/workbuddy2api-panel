package panel

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linguo2625469/workbuddy2api-panel/internal/auth"
	"github.com/linguo2625469/workbuddy2api-panel/internal/pool"
	"github.com/linguo2625469/workbuddy2api-panel/internal/upstream"
)

// TestSchoolVouchersHandler 钉住「查询券码」的后端契约：
//   - 正常 CN 号把上游 data.items 原样透出（券码本体、奖品名、有效期）；
//   - 禁用号直接跳过，不发上游调用、也不出现在响应里；
//   - 上游报错只标在该行 error 上，不影响其它账号（不整体失败）。
func TestSchoolVouchersHandler(t *testing.T) {
	var cnCalls, failCalls int
	const okBody = `{"code":0,"msg":"OK","data":{"items":[` +
		`{"grant_id":11,"sku_code":"voucher_luckin","prize_name":"瑞幸咖啡","code":"LK-2027","valid_to":"2027-09-30"},` +
		`{"grant_id":12,"sku_code":"kfc_ice_cream","prize_name":"肯德基冰淇淋","code":"KFC-1024","valid_to":"2026-10-24"}]}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/portal/activity/school/vouchers") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Header.Get("X-User-Id") {
		case "cn-ok":
			cnCalls++
			_, _ = io.WriteString(w, okBody)
		case "cn-fail":
			failCalls++
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"code":500,"msg":"boom"}`)
		default:
			http.Error(w, "unexpected uid", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	accounts := pool.New("")
	accounts.Add(&auth.Auth{UID: "cn-ok", Nickname: "正常号", AccessToken: "at"})
	accounts.Add(&auth.Auth{UID: "cn-fail", Nickname: "报错号", AccessToken: "at"})
	accounts.Add(&auth.Auth{UID: "cn-disabled", Nickname: "禁用号", AccessToken: "at"})
	accounts.Disable("cn-disabled", "test")

	client := upstream.New()
	client.HTTP = &http.Client{Timeout: 5 * time.Second}
	client.BillingBaseCN = srv.URL
	p := New(Config{Pool: accounts, Upstream: client})

	rec := httptest.NewRecorder()
	p.schoolVouchers(rec, httptest.NewRequest("GET", "/panel/api/school/vouchers", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Accounts []struct {
			UID      string `json:"uid"`
			Nickname string `json:"nickname"`
			Vouchers []struct {
				GrantID   int64  `json:"grant_id"`
				SKUCode   string `json:"sku_code"`
				PrizeName string `json:"prize_name"`
				Code      string `json:"code"`
				ValidTo   string `json:"valid_to"`
			} `json:"vouchers"`
			Err string `json:"error"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resp not json: %v body=%s", err, rec.Body.String())
	}
	if len(resp.Accounts) != 2 {
		t.Fatalf("应只回 2 个未禁用账号（禁用号跳过），实际 %d: %+v", len(resp.Accounts), resp.Accounts)
	}

	byUID := map[string]int{}
	for i, row := range resp.Accounts {
		byUID[row.UID] = i
	}
	if _, ok := byUID["cn-disabled"]; ok {
		t.Error("禁用号不该出现在响应里")
	}

	okIdx, ok := byUID["cn-ok"]
	if !ok {
		t.Fatal("正常号缺失")
	}
	okRow := resp.Accounts[okIdx]
	if okRow.Err != "" {
		t.Errorf("正常号不该有 error: %q", okRow.Err)
	}
	if len(okRow.Vouchers) != 2 {
		t.Fatalf("正常号应有 2 张券，实际 %d", len(okRow.Vouchers))
	}
	if okRow.Vouchers[0].Code != "LK-2027" || okRow.Vouchers[0].PrizeName != "瑞幸咖啡" ||
		okRow.Vouchers[0].SKUCode != "voucher_luckin" || okRow.Vouchers[0].ValidTo != "2027-09-30" {
		t.Errorf("券字段透出不对: %+v", okRow.Vouchers[0])
	}
	if okRow.Vouchers[0].GrantID != 11 {
		t.Errorf("grant_id=%d want 11", okRow.Vouchers[0].GrantID)
	}

	failIdx, ok := byUID["cn-fail"]
	if !ok {
		t.Fatal("报错号缺失")
	}
	if resp.Accounts[failIdx].Err == "" {
		t.Error("上游 500 的账号必须带 error（不能静默当成没券）")
	}
	if len(resp.Accounts[failIdx].Vouchers) != 0 {
		t.Errorf("失败账号不该有券: %+v", resp.Accounts[failIdx].Vouchers)
	}

	if cnCalls != 1 || failCalls != 1 {
		t.Errorf("上游调用次数不对: ok=%d fail=%d（禁用号必须一次都不打）", cnCalls, failCalls)
	}
}
