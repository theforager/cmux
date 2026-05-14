package tmux

import "testing"

func TestFirstUnixUsesFirstPositiveTimestamp(t *testing.T) {
	parts := []string{"", "0", "123", "456"}
	if got := firstUnix(parts, 0, 1, 2, 3); got != 123 {
		t.Fatalf("firstUnix = %d, want 123", got)
	}
}

func TestFirstUnixReturnsZeroWhenMissing(t *testing.T) {
	if got := firstUnix([]string{"", "not-time"}, 0, 1, 2); got != 0 {
		t.Fatalf("firstUnix = %d, want 0", got)
	}
}
