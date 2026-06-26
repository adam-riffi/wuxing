package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewRootCmd_Tree(t *testing.T) {
	root := newRootCmd()

	if root.Use != "wxg" {
		t.Errorf("root.Use: got %q want wxg", root.Use)
	}

	lib, _, err := root.Find([]string{"library"})
	if err != nil || lib.Name() != "library" {
		t.Fatalf("library command not found: %v", err)
	}

	want := []string{"index", "deindex", "status", "calls", "functions"}
	got := make(map[string]bool)
	for _, c := range lib.Commands() {
		got[c.Name()] = true
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("library missing subcommand %q", name)
		}
	}
}

func TestLibrarySubcommand_StubError(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"library", "index", "mtg"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected stub to return an error, got nil")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("error %q does not mention 'not implemented'", err.Error())
	}
}
