package setup

import "testing"

func TestWebHostLabelOnlyForThePrimaryDomain(t *testing.T) {
	cases := []struct {
		web, primary string
		want         string
	}{
		{"mail.example.com", "example.com", "mail"},
		{"example.com", "example.com", ""},
		{"mail.other.com", "example.com", ""},
		{"MAIL.Example.COM.", "example.com", "mail"},
	}
	for _, tc := range cases {
		if got := webHostLabel(tc.web, tc.primary); got != tc.want {
			t.Errorf("webHostLabel(%q, %q) = %q, want %q", tc.web, tc.primary, got, tc.want)
		}
	}
}
