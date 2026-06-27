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

func TestInferCmd_Tree(t *testing.T) {
	root := newRootCmd()
	infer, _, err := root.Find([]string{"infer"})
	if err != nil || infer.Name() != "infer" {
		t.Fatalf("infer command not found: %v", err)
	}
	got := make(map[string]bool)
	for _, c := range infer.Commands() {
		got[c.Name()] = true
	}
	if !got["chat"] || !got["agent"] {
		t.Errorf("infer should have chat + agent subcommands, got %v", got)
	}
}

func TestInferChat_UnknownBackend(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	// "bogus" is rejected before any CLI is spawned, so this is hermetic.
	root.SetArgs([]string{"infer", "chat", "hi", "bogus"})

	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "unknown backend") {
		t.Errorf("expected an unknown-backend error, got %v", err)
	}
}
