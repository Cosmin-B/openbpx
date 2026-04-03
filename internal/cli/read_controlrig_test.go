package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunControlRigReadAcceptsFlags(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{"controlrig", "read", "/tmp/nonexistent.uasset", "--include-properties=false"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code: got %d want 1", code)
	}
	if strings.Contains(stderr.String(), "usage: bpx controlrig read") {
		t.Fatalf("unexpected usage error, flags likely not parsed: %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "read file") {
		t.Fatalf("expected file read failure, got: %s", stderr.String())
	}
}
