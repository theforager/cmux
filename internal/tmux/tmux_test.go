package tmux

import "testing"

func TestIsNoServerErrorRecognizesMissingSocket(t *testing.T) {
	err := errString("tmux list-sessions -F #{session_name} failed: error connecting to /private/tmp/tmux-502/default (No such file or directory)")
	if !IsNoServerError(err) {
		t.Fatal("missing tmux socket should be treated as no server")
	}
}

func TestIsNoServerErrorRecognizesNoServerRunning(t *testing.T) {
	err := errString("tmux list-sessions failed: no server running on /private/tmp/tmux-502/default")
	if !IsNoServerError(err) {
		t.Fatal("no server running should be treated as no server")
	}
}

func TestIsNoServerErrorDoesNotHideOtherTmuxErrors(t *testing.T) {
	err := errString("tmux list-sessions failed: unknown option: -Z")
	if IsNoServerError(err) {
		t.Fatal("unrelated tmux errors should still be reported")
	}
}

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

type errString string

func (e errString) Error() string {
	return string(e)
}
