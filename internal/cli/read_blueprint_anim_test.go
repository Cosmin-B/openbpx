package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wilddogjp/openbpx/pkg/uasset"
)

func TestRunBlueprintAnimInfoAcceptsFlags(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{"blueprint", "anim-info", "/tmp/nonexistent.uasset", "--include-properties=false"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code: got %d want 1", code)
	}
	if strings.Contains(stderr.String(), "usage: bpx blueprint anim-info") {
		t.Fatalf("unexpected usage error, flags likely not parsed: %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "read file") {
		t.Fatalf("expected file read failure, got: %s", stderr.String())
	}
}

func TestAnimBlueprintPrimaryCDOIndexFindsDefaultObject(t *testing.T) {
	asset := testAssetWithExports([]testExportSpec{
		{ObjectName: "BP_Test", ClassName: "Blueprint"},
		{ObjectName: "Default__BP_Test_C", ClassName: "BP_Test_C"},
	})
	if got := animBlueprintPrimaryCDOIndex(asset); got != 1 {
		t.Fatalf("animBlueprintPrimaryCDOIndex: got %d want 1", got)
	}
}

type testExportSpec struct {
	ObjectName string
	ClassName  string
}

func testAssetWithExports(specs []testExportSpec) *uasset.Asset {
	names := []uasset.NameEntry{{Value: "None"}}
	nameIndex := map[string]int32{"None": 0}
	addName := func(value string) int32 {
		if idx, ok := nameIndex[value]; ok {
			return idx
		}
		idx := int32(len(names))
		nameIndex[value] = idx
		names = append(names, uasset.NameEntry{Value: value})
		return idx
	}

	imports := make([]uasset.ImportEntry, 0, len(specs))
	exports := make([]uasset.ExportEntry, 0, len(specs))
	for _, spec := range specs {
		classImportIndex := int32(len(imports) + 1)
		imports = append(imports, uasset.ImportEntry{
			ObjectName: uasset.NameRef{Index: addName(spec.ClassName)},
		})
		exports = append(exports, uasset.ExportEntry{
			ClassIndex: uasset.PackageIndex(-classImportIndex),
			ObjectName: uasset.NameRef{Index: addName(spec.ObjectName)},
		})
	}

	return &uasset.Asset{
		Names:   names,
		Imports: imports,
		Exports: exports,
	}
}
