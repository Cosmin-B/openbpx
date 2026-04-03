package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/wilddogjp/openbpx/pkg/uasset"
)

func runBlueprintAnimInfo(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("blueprint anim-info", stderr)
	opts := registerCommonFlags(fs)
	includeProperties := fs.Bool("include-properties", false, "include decoded property payloads for key exports")
	if err := parseFlagSet(fs, args); err != nil {
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: bpx blueprint anim-info <file.uasset> [--include-properties]")
		return 1
	}

	file := fs.Arg(0)
	asset, err := uasset.ParseFile(file, *opts)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	payload, err := buildAnimBlueprintInfoPayload(file, asset, *includeProperties)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return printJSON(stdout, payload)
}

func buildAnimBlueprintInfoPayload(file string, asset *uasset.Asset, includeProperties bool) (map[string]any, error) {
	if asset == nil {
		return nil, fmt.Errorf("asset is nil")
	}

	graphChildCounts := childExportCountsByOuter(asset)
	animationGraphs := make([]map[string]any, 0, 32)
	stateGraphs := make([]map[string]any, 0, 32)
	stateMachineGraphs := make([]map[string]any, 0, 16)
	transitionGraphs := make([]map[string]any, 0, 64)
	generatedStructs := make([]map[string]any, 0, 8)
	generatedClasses := make([]map[string]any, 0, 4)
	blueprintExports := make([]map[string]any, 0, 4)

	for idx, exp := range asset.Exports {
		className := asset.ResolveClassName(exp)
		switch {
		case strings.EqualFold(className, "AnimationGraph"):
			animationGraphs = append(animationGraphs, buildAnimGraphEntry(asset, idx, graphChildCounts[idx+1], includeProperties))
		case strings.EqualFold(className, "AnimationStateGraph"):
			stateGraphs = append(stateGraphs, buildAnimGraphEntry(asset, idx, graphChildCounts[idx+1], includeProperties))
		case strings.EqualFold(className, "AnimationStateMachineGraph"):
			stateMachineGraphs = append(stateMachineGraphs, buildAnimGraphEntry(asset, idx, graphChildCounts[idx+1], includeProperties))
		case strings.EqualFold(className, "AnimationTransitionGraph"):
			transitionGraphs = append(transitionGraphs, buildAnimGraphEntry(asset, idx, graphChildCounts[idx+1], includeProperties))
		case strings.EqualFold(className, "AnimBlueprintGeneratedClass"):
			generatedClasses = append(generatedClasses, buildBlueprintExportSummary(asset, idx))
		case strings.EqualFold(className, "ScriptStruct") && strings.Contains(strings.ToLower(exp.ObjectName.Display(asset.Names)), "animblueprintgenerated"):
			generatedStructs = append(generatedStructs, buildBlueprintExportSummary(asset, idx))
		case strings.EqualFold(className, "Blueprint"):
			blueprintExports = append(blueprintExports, buildBlueprintExportSummary(asset, idx))
		}
	}

	sortNiagaraEntries(animationGraphs)
	sortNiagaraEntries(stateGraphs)
	sortNiagaraEntries(stateMachineGraphs)
	sortNiagaraEntries(transitionGraphs)
	sortNiagaraEntries(generatedStructs)
	sortNiagaraEntries(generatedClasses)
	sortNiagaraEntries(blueprintExports)

	cdoIndex := animBlueprintPrimaryCDOIndex(asset)
	cdoSummary := map[string]any(nil)
	if cdoIndex >= 0 {
		cdoSummary = buildAnimBlueprintCDOSummary(asset, cdoIndex, includeProperties)
	}

	return map[string]any{
		"file":                   file,
		"includeProperties":      includeProperties,
		"blueprintExportCount":   len(blueprintExports),
		"generatedClassCount":    len(generatedClasses),
		"animationGraphCount":    len(animationGraphs),
		"stateGraphCount":        len(stateGraphs),
		"stateMachineGraphCount": len(stateMachineGraphs),
		"transitionGraphCount":   len(transitionGraphs),
		"generatedStructCount":   len(generatedStructs),
		"blueprintExports":       blueprintExports,
		"generatedClasses":       generatedClasses,
		"animationGraphs":        animationGraphs,
		"stateGraphs":            stateGraphs,
		"stateMachineGraphs":     stateMachineGraphs,
		"transitionGraphs":       transitionGraphs,
		"generatedStructs":       generatedStructs,
		"cdoSummary":             cdoSummary,
		"mentalModel": map[string]any{
			"summary": "AnimBlueprint assets are Blueprint packages that embed AnimationGraph, state machine, and transition graph exports alongside generated anim blueprint data structs and CDO anim node properties.",
			"layers": []string{
				"AnimationGraph / AnimationStateGraph / AnimationStateMachineGraph / AnimationTransitionGraph exports represent editor-side graph structure.",
				"The CDO often carries serialized AnimGraphNode_* properties that reflect runtime node instances and generated mutable/constant anim data.",
				"Generated anim blueprint script structs capture constant and mutable runtime data emitted by compilation.",
			},
		},
	}, nil
}

func animBlueprintPrimaryCDOIndex(asset *uasset.Asset) int {
	if asset == nil {
		return -1
	}
	for idx, exp := range asset.Exports {
		if strings.HasPrefix(exp.ObjectName.Display(asset.Names), "Default__") {
			return idx
		}
	}
	return -1
}

func buildAnimBlueprintCDOSummary(asset *uasset.Asset, exportIndex int, includeProperties bool) map[string]any {
	exp := asset.Exports[exportIndex]
	props := asset.ParseExportProperties(exportIndex)
	propMap := decodedPropertyMap(asset, props.Properties)
	animNodeProps := make([]map[string]any, 0, 64)
	animExtensions := make([]string, 0, 8)
	controlRigProps := make([]string, 0, 8)

	for _, p := range props.Properties {
		name := p.Name.Display(asset.Names)
		switch {
		case strings.HasPrefix(name, "AnimGraphNode_"):
			animNodeProps = append(animNodeProps, map[string]any{
				"name": name,
				"type": p.TypeString(asset.Names),
			})
		case strings.HasPrefix(name, "AnimBlueprintExtension_"):
			animExtensions = append(animExtensions, name)
		case strings.Contains(strings.ToLower(name), "controlrig"):
			controlRigProps = append(controlRigProps, name)
		}
	}

	sort.Slice(animNodeProps, func(i, j int) bool {
		left, _ := animNodeProps[i]["name"].(string)
		right, _ := animNodeProps[j]["name"].(string)
		return left < right
	})
	sort.Strings(animExtensions)
	sort.Strings(controlRigProps)

	entry := map[string]any{
		"export":                  exportIndex + 1,
		"objectName":              exp.ObjectName.Display(asset.Names),
		"className":               asset.ResolveClassName(exp),
		"animNodePropertyCount":   len(animNodeProps),
		"animNodeProperties":      animNodeProps,
		"animBlueprintExtensions": animExtensions,
		"controlRigProperties":    controlRigProps,
		"generatedMutableData":    summarizeStructuredValue(propMap["__AnimBlueprintMutables"]),
	}
	if len(props.Warnings) > 0 {
		entry["warnings"] = append([]string(nil), props.Warnings...)
	}
	if includeProperties {
		entry["properties"] = toPropertyOutputs(asset, props.Properties, true)
	}
	return entry
}

func buildAnimGraphEntry(asset *uasset.Asset, exportIndex int, childCount int, includeProperties bool) map[string]any {
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

func buildBlueprintExportSummary(asset *uasset.Asset, exportIndex int) map[string]any {
	exp := asset.Exports[exportIndex]
	return map[string]any{
		"export":     exportIndex + 1,
		"objectName": exp.ObjectName.Display(asset.Names),
		"className":  asset.ResolveClassName(exp),
		"outerIndex": int32(exp.OuterIndex),
	}
}
