package format

import (
	"testing"
	"time"
)

func TestAgeShowsSecondsBeforeOneMinute(t *testing.T) {
	if got := age(9 * time.Second); got != "now" {
		t.Fatalf("age(9s) = %q, want now", got)
	}
	if got := age(42 * time.Second); got != "42s" {
		t.Fatalf("age(42s) = %q, want 42s", got)
	}
	if got := age(2 * time.Minute); got != "2m" {
		t.Fatalf("age(2m) = %q, want 2m", got)
	}
}
