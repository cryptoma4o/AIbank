package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)
	if code != 2 {
		t.Errorf("want exit 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "USAGE:") {
		t.Errorf("usage missing from stderr: %s", stderr.String())
	}
}

func TestRun_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"frobnicate"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("want exit 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown subcommand") {
		t.Errorf("expected unknown-subcommand message; stderr=%q", stderr.String())
	}
}

func TestRun_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--help"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("want exit 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "USAGE:") {
		t.Errorf("usage missing from stdout: %s", stdout.String())
	}
}

func TestRun_CreateMissingFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"create", "--id", "demo"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("want exit 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "required") {
		t.Errorf("expected required-flags error; stderr=%q", stderr.String())
	}
}

func TestRun_GetMissingID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"get"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("want exit 2, got %d", code)
	}
}
