package tui

import (
	"reflect"
	"testing"
)

func TestSplitEditorCommand(t *testing.T) {
	got, err := splitEditorCommand(`open -a "Visual Studio Code"`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"open", "-a", "Visual Studio Code"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitEditorCommand = %#v, want %#v", got, want)
	}
}

func TestSplitEditorCommandRejectsUnterminatedQuote(t *testing.T) {
	if _, err := splitEditorCommand(`open -a "Visual Studio Code`); err == nil {
		t.Fatal("expected unterminated quote error")
	}
}
