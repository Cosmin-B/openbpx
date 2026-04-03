package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunNiagaraReadAcceptsFlags(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{"niagara", "read", "/tmp/nonexistent.uasset", "--export", "1", "--include-properties=false"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code: got %d want 1", code)
	}
	if strings.Contains(stderr.String(), "usage: bpx niagara read") {
		t.Fatalf("unexpected usage error, flags likely not parsed: %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "read file") {
		t.Fatalf("expected file read failure, got: %s", stderr.String())
	}
}

func TestNiagaraClassifiers(t *testing.T) {
	if !isNiagaraSystemClassName("NiagaraSystem") {
		t.Fatal("expected NiagaraSystem classifier match")
	}
	if !isNiagaraEmitterClassName("NiagaraEmitter") {
		t.Fatal("expected NiagaraEmitter classifier match")
	}
	if !isNiagaraGraphClassName("NiagaraGraph") {
		t.Fatal("expected NiagaraGraph classifier match")
	}
	if !isNiagaraScriptClassName("NiagaraScript") {
		t.Fatal("expected NiagaraScript classifier match")
	}
	if !isNiagaraRendererClassName("NiagaraSpriteRendererProperties") {
		t.Fatal("expected Niagara renderer classifier match")
	}
	if !isNiagaraDataInterfaceClassName("NiagaraDataInterfaceCurve") {
		t.Fatal("expected Niagara data interface classifier match")
	}
	if !isNiagaraScriptVariableClassName("NiagaraScriptVariable") {
		t.Fatal("expected Niagara script variable classifier match")
	}
}

func TestBuildNiagaraMentalModelIncludesCounts(t *testing.T) {
	model := buildNiagaraMentalModel(
		[]map[string]any{{"objectName": "System"}},
		[]map[string]any{{"objectName": "Emitter"}},
		[]map[string]any{{"objectName": "Script"}},
		[]map[string]any{{"objectName": "Renderer"}},
		[]map[string]any{{"objectName": "DI"}},
		[]map[string]any{{"objectName": "Var"}},
	)
	counts, ok := model["counts"].(map[string]any)
	if !ok {
		t.Fatalf("counts payload: %#v", model["counts"])
	}
	if got, want := counts["systems"], 1; got != want {
		t.Fatalf("systems count: got %v want %v", got, want)
	}
}
