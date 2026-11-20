package imap_server

import "testing"

func TestMailboxMatchWildcards(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		match   bool
	}{
		{"INBOX", "inbox", true},
		{"INBOX", "*", true},
		{"INBOX", "%", true},
		{"Parent/Child", "Parent/%", true},
		{"Parent/Child/Grandchild", "Parent/%", false},
		{"Parent/Child/Grandchild", "Parent/*", true},
		{"Parent/Child", "Other/%", false},
		{"Sent Messages", "Sent*", true},
		{"", "", true},
	}
	for _, tc := range cases {
		if got := mailboxMatch(tc.name, tc.pattern); got != tc.match {
			t.Errorf("mailboxMatch(%q, %q) = %v, want %v", tc.name, tc.pattern, got, tc.match)
		}
	}
}
