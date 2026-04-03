package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunPCGReadAcceptsFlags(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{"pcg", "read", "/tmp/nonexistent.uasset", "--export", "1", "--include-properties=false"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code: got %d want 1", code)
	}
	if strings.Contains(stderr.String(), "usage: bpx pcg read") {
		t.Fatalf("unexpected usage error, flags likely not parsed: %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "read file") {
		t.Fatalf("expected file read failure, got: %s", stderr.String())
	}
}

func TestPCGClassifiers(t *testing.T) {
	if !isPCGGraphClassName("PCGGraph") {
		t.Fatal("expected PCGGraph classifier match")
	}
	if !isPCGNodeClassName("PCGNode") {
		t.Fatal("expected PCGNode classifier match")
	}
	if !isPCGSettingsClassName("PCGStaticMeshSpawnerSettings") {
		t.Fatal("expected PCG settings classifier match")
	}
}

