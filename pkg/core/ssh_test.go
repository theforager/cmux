package core

import "testing"

func TestParseSSHHosts(t *testing.T) {
	got := ParseSSHHosts(`
# comment
dev=dev@example.com
prod=-p 2222 user@prod.example.com
bad
`, "/tmp/hosts")
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(got), got)
	}
	if got[0].Alias != "dev" || got[0].Target != "dev@example.com" {
		t.Fatalf("first host = %+v", got[0])
	}
	if got[1].Alias != "prod" || got[1].Target != "user@prod.example.com" || len(got[1].Args) != 3 {
		t.Fatalf("second host = %+v", got[1])
	}
}
