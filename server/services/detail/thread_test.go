package detail

import "testing"

func TestNormalizeThreadSubject(t *testing.T) {
	cases := map[string]string{
		"Re: Hello":        "hello",
		"FWD: RE:  Hello ": "hello",
		"回覆: 測試郵件":         "測試郵件",
		"轉寄: Fwd: 專案進度":    "專案進度",
		"AW: Re: Status":   "status",
		"ไม่มีคำนำหน้า":    "ไม่มีคำนำหน้า",
		"":    "",
		"   ": "",
	}
	for input, want := range cases {
		if got := NormalizeThreadSubject(input); got != want {
			t.Fatalf("NormalizeThreadSubject(%q) = %q, want %q", input, got, want)
		}
	}
}
