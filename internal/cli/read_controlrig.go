package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/wilddogjp/openbpx/pkg/uasset"
)

func runControlRig(args []string, stdout, stderr io.Writer) int {
	return dispatchSubcommand(
		args,
		stdout,
		stderr,
		"usage: bpx controlrig <read> ...",
		"unknown controlrig command: %s\n",
		subcommandSpec{Name: "read", Run: runControlRigRead},
	)
}

func runControlRigRead(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("controlrig read", stderr)
	opts := registerCommonFlags(fs)
	includeProperties := fs.Bool("include-properties", false, "include decoded property payloads for key ControlRig exports")
	if err := parseFlagSet(fs, args); err != nil {
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: bpx controlrig read <file.uasset> [--include-properties]")
		return 1
	}

	file := fs.Arg(0)
	asset, err := uasset.ParseFile(file, *opts)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	payload, err := buildControlRigReadPayload(file, asset, *includeProperties)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return printJSON(stdout, payload)
}

func buildControlRigReadPayload(file string, asset *uasset.Asset, includeProperties bool) (map[string]any, error) {
	if asset == nil {
		return nil, fmt.Errorf("asset is nil")
	}
	graphChildCounts := childExportCountsByOuter(asset)
	graphs := make([]map[string]any, 0, 64)
	graphNodeCount := 0
	rigVMVariableNodeCount := 0
	blueprints := make([]map[string]any, 0, 2)
	generatedClasses := make([]map[string]any, 0, 2)

	for idx, exp := range asset.Exports {
		className := asset.ResolveClassName(exp)
		switch {
		case strings.EqualFold(className, "ControlRigBlueprint"):
			blueprints = append(blueprints, buildControlRigBlueprintEntry(asset, idx, includeProperties))
		case strings.EqualFold(className, "ControlRigBlueprintGeneratedClass"):
			generatedClasses = append(generatedClasses, buildBlueprintExportSummary(asset, idx))
		case strings.EqualFold(className, "ControlRigGraph"):
			entry := buildControlRigGraphEntry(asset, idx, graphChildCounts[idx+1], includeProperties)
			graphs = append(graphs, entry)
		case strings.EqualFold(className, "ControlRigGraphNode"):
			graphNodeCount++
		case strings.EqualFold(className, "RigVMVariableNode"):
			rigVMVariableNodeCount++
		}
	}

	sortNiagaraEntries(blueprints)
	sortNiagaraEntries(generatedClasses)
	sortNiagaraEntries(graphs)

	return map[string]any{
		"file":                   file,
		"includeProperties":      includeProperties,
		"blueprintCount":         len(blueprints),
		"generatedClassCount":    len(generatedClasses),
		"graphCount":             len(graphs),
		"graphNodeCount":         graphNodeCount,
		"rigVMVariableNodeCount": rigVMVariableNodeCount,
		"blueprints":             blueprints,
		"generatedClasses":       generatedClasses,
		"graphs":                 graphs,
		"mentalModel": map[string]any{
			"summary": "A ControlRig package is centered on a ControlRigBlueprint plus one or more ControlRigGraph exports, generated class data, and large numbers of ControlRigGraphNode and RigVMVariableNode exports representing the editable rig VM model.",
			"layers": []string{
				"ControlRigBlueprint stores hierarchy references, preview/source skeleton references, RigVM client/model references, and function reference metadata.",
				"ControlRigGraph exports represent functions or rig graphs; their child exports are the graph nodes.",
				"RigVMVariableNode exports represent the variable-side VM graph model alongside ControlRigGraphNode exports.",
			},
		},
	}, nil
}

func buildControlRigBlueprintEntry(asset *uasset.Asset, exportIndex int, includeProperties bool) map[string]any {
	exp := asset.Exports[exportIndex]
	props := asset.ParseExportProperties(exportIndex)
	propMap := decodedPropertyMap(asset, props.Properties)
	entry := map[string]any{
		"export":                     exportIndex + 1,
		"objectName":                 exp.ObjectName.Display(asset.Names),
		"className":                  asset.ResolveClassName(exp),
		"hierarchy":                  decodeObjectReference(propMap["Hierarchy"]),
		"previewSkeletalMesh":        decodeObjectReference(propMap["PreviewSkeletalMesh"]),
		"sourceHierarchyImport":      decodeObjectReference(propMap["SourceHierarchyImport"]),
		"sourceCurveImport":          decodeObjectReference(propMap["SourceCurveImport"]),
		"validator":                  decodeObjectReference(propMap["Validator"]),
		"functionLibraryEdGraph":     decodeObjectReference(propMap["FunctionLibraryEdGraph"]),
		"generatedClass":             decodeObjectReference(propMap["GeneratedClass"]),
		"rigVMClient":                summarizeControlRigVMClient(propMap["RigVMClient"]),
		"functionReferenceNodeCount": decodeWrappedArrayCount(propMap["FunctionReferenceNodeData"]),
		"categorySortingCount":       decodeWrappedArrayCount(propMap["CategorySorting"]),
	}
	if len(props.Warnings) > 0 {
		entry["warnings"] = append([]string(nil), props.Warnings...)
	}
	if includeProperties {
		entry["properties"] = toPropertyOutputs(asset, props.Properties, true)
	}
	return entry
}

func summarizeControlRigVMClient(value any) map[string]any {
	fields, ok := unwrapStructValue(value)
	if !ok {
		return nil
	}
	out := map[string]any{
		"functionLibrary": decodeObjectReference(fields["FunctionLibrary"]),
	}
	if models := decodeObjectReferenceArray(fields["Models"]); len(models) > 0 {
		out["models"] = models
		out["modelCount"] = len(models)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func buildControlRigGraphEntry(asset *uasset.Asset, exportIndex int, childCount int, includeProperties bool) map[string]any {
	exp := asset.Exports[exportIndex]
	entry := map[string]any{
		"export":       exportIndex + 1,
		"objectName":   exp.ObjectName.Display(asset.Names),
		"className":    asset.ResolveClassName(exp),
		"outerIndex":   int32(exp.OuterIndex),
		"childExports": childCount,
	}
	if includeProperties {
		props := asset.ParseExportProperties(exportIndex)
		entry["properties"] = toPropertyOutputs(asset, props.Properties, true)
		if len(props.Warnings) > 0 {
			entry["warnings"] = append([]string(nil), props.Warnings...)
		}
	}
	return entry
}
