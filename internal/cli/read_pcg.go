package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/wilddogjp/openbpx/pkg/uasset"
)

func runPCG(args []string, stdout, stderr io.Writer) int {
	return dispatchSubcommand(
		args,
		stdout,
		stderr,
		"usage: bpx pcg <read> ...",
		"unknown pcg command: %s\n",
		subcommandSpec{Name: "read", Run: runPCGRead},
	)
}

func runPCGRead(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("pcg read", stderr)
	opts := registerCommonFlags(fs)
	exportIndex := fs.Int("export", 0, "optional 1-based PCGGraph export index")
	includeProperties := fs.Bool("include-properties", false, "include decoded property payloads for summarized PCG exports")
	if err := parseFlagSet(fs, args); err != nil {
		return 1
	}
	if fs.NArg() != 1 || *exportIndex < 0 {
		fmt.Fprintln(stderr, "usage: bpx pcg read <file.uasset> [--export <n>] [--include-properties]")
		return 1
	}

	file := fs.Arg(0)
	asset, err := uasset.ParseFile(file, *opts)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	payload, err := buildPCGReadPayload(file, asset, *exportIndex, *includeProperties)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return printJSON(stdout, payload)
}

func buildPCGReadPayload(file string, asset *uasset.Asset, exportFilter int, includeProperties bool) (map[string]any, error) {
	if asset == nil {
		return nil, fmt.Errorf("asset is nil")
	}
	graphTargets, err := pcgGraphTargetExports(asset, exportFilter)
	if err != nil {
		return nil, err
	}

	graphEntries := make([]map[string]any, 0, len(graphTargets))
	nodeEntries := make([]map[string]any, 0, 32)
	settingsEntries := make([]map[string]any, 0, 32)
	pinEntries := make([]map[string]any, 0, 64)
	edgeEntries := make([]map[string]any, 0, 64)
	pointDataEntries := make([]map[string]any, 0, 16)
	metadataEntries := make([]map[string]any, 0, 16)
	blueprintSettingsEntries := make([]map[string]any, 0, 16)

	nodeSettingsByExport := map[int]map[string]any{}
	pinsByNodeExport := map[int][]map[string]any{}
	edgesByNodeExport := map[int][]map[string]any{}

	for idx, exp := range asset.Exports {
		className := asset.ResolveClassName(exp)
		switch {
		case isPCGGraphClassName(className):
			graphEntries = append(graphEntries, buildPCGGraphEntry(asset, idx, includeProperties))
		case isPCGNodeClassName(className):
			entry := buildPCGNodeEntry(asset, idx, includeProperties)
			nodeEntries = append(nodeEntries, entry)
			if settings, ok := entry["settings"].(map[string]any); ok && len(settings) > 0 {
				nodeSettingsByExport[idx+1] = settings
			}
		case isPCGSettingsClassName(className):
			settingsEntries = append(settingsEntries, buildPCGSimpleEntry(asset, idx, includeProperties))
		case strings.EqualFold(className, "PCGPin"):
			entry := buildPCGPinEntry(asset, idx, includeProperties)
			pinEntries = append(pinEntries, entry)
			if outer, ok := entry["outerIndex"].(int32); ok && outer > 0 {
				pinsByNodeExport[int(outer)] = append(pinsByNodeExport[int(outer)], entry)
			}
		case strings.EqualFold(className, "PCGEdge"):
			entry := buildPCGSimpleEntry(asset, idx, includeProperties)
			edgeEntries = append(edgeEntries, entry)
			if outer, ok := entry["outerIndex"].(int32); ok && outer > 0 {
				edgesByNodeExport[int(outer)] = append(edgesByNodeExport[int(outer)], entry)
			}
		case strings.EqualFold(className, "PCGPointData"):
			pointDataEntries = append(pointDataEntries, buildPCGSimpleEntry(asset, idx, includeProperties))
		case strings.EqualFold(className, "PCGMetadata"):
			metadataEntries = append(metadataEntries, buildPCGSimpleEntry(asset, idx, includeProperties))
		case strings.EqualFold(className, "PCGBlueprintSettings"):
			blueprintSettingsEntries = append(blueprintSettingsEntries, buildPCGSimpleEntry(asset, idx, includeProperties))
		}
	}

	sortNiagaraEntries(graphEntries)
	sortNiagaraEntries(nodeEntries)
	sortNiagaraEntries(settingsEntries)
	sortNiagaraEntries(pinEntries)
	sortNiagaraEntries(edgeEntries)
	sortNiagaraEntries(pointDataEntries)
	sortNiagaraEntries(metadataEntries)
	sortNiagaraEntries(blueprintSettingsEntries)

	for _, graphEntry := range graphEntries {
		if exportNum, ok := graphEntry["export"].(int); ok {
			if nodes, ok := graphEntry["nodes"].([]map[string]any); ok {
				for _, node := range nodes {
					if nodeExport, ok := node["export"].(int); ok {
						node["settings"] = nodeSettingsByExport[nodeExport]
						node["pins"] = pinsByNodeExport[nodeExport]
						node["edges"] = edgesByNodeExport[nodeExport]
					}
				}
			}
			graphEntry["nodeCount"] = len(graphEntry["nodes"].([]map[string]any))
			graphEntry["pinCount"] = len(flattenPCGChildEntries(graphEntry["nodes"], "pins"))
			graphEntry["edgeCount"] = len(flattenPCGChildEntries(graphEntry["nodes"], "edges"))
			_ = exportNum
		}
	}

	return map[string]any{
		"file":                   file,
		"exportFilter":           exportFilter,
		"includeProperties":      includeProperties,
		"graphCount":             len(graphEntries),
		"nodeCount":              len(nodeEntries),
		"settingsCount":          len(settingsEntries),
		"pinCount":               len(pinEntries),
		"edgeCount":              len(edgeEntries),
		"pointDataCount":         len(pointDataEntries),
		"metadataCount":          len(metadataEntries),
		"blueprintSettingsCount": len(blueprintSettingsEntries),
		"graphs":                 graphEntries,
		"nodes":                  nodeEntries,
		"settings":               settingsEntries,
		"pins":                   pinEntries,
		"edges":                  edgeEntries,
		"pointData":              pointDataEntries,
		"metadata":               metadataEntries,
		"blueprintSettings":      blueprintSettingsEntries,
		"mentalModel": map[string]any{
			"summary": "A PCG asset is typically centered on one PCGGraph export that points to PCGNode exports. Each node references a settings export and owns input/output PCGPin exports, while PCGEdge exports model connectivity and point/metadata exports capture generated data-side state.",
			"layers": []string{
				"PCGGraph holds the top-level Nodes array plus default input/output nodes.",
				"PCGNode exports carry editor position and SettingsInterface references to the concrete PCG settings object used by the node.",
				"PCGPin and PCGEdge exports represent graph connectivity, while settings exports such as PCGStaticMeshSpawnerSettings and PCGSplineSamplerSettings carry behavior-specific configuration.",
			},
		},
	}, nil
}

func buildPCGGraphEntry(asset *uasset.Asset, exportIndex int, includeProperties bool) map[string]any {
	exp := asset.Exports[exportIndex]
	props := asset.ParseExportProperties(exportIndex)
	propMap := decodedPropertyMap(asset, props.Properties)
	entry := map[string]any{
		"export":     exportIndex + 1,
		"objectName": exp.ObjectName.Display(asset.Names),
		"className":  asset.ResolveClassName(exp),
		"nodes":      decodePCGObjectRefs(propMap["Nodes"]),
		"inputNode":  decodeObjectReference(propMap["InputNode"]),
		"outputNode": decodeObjectReference(propMap["OutputNode"]),
	}
	if len(props.Warnings) > 0 {
		entry["warnings"] = append([]string(nil), props.Warnings...)
	}
	if includeProperties {
		entry["properties"] = toPropertyOutputs(asset, props.Properties, true)
	}
	return entry
}

func buildPCGNodeEntry(asset *uasset.Asset, exportIndex int, includeProperties bool) map[string]any {
	exp := asset.Exports[exportIndex]
	props := asset.ParseExportProperties(exportIndex)
	propMap := decodedPropertyMap(asset, props.Properties)
	entry := map[string]any{
		"export":            exportIndex + 1,
		"objectName":        exp.ObjectName.Display(asset.Names),
		"className":         asset.ResolveClassName(exp),
		"outerIndex":        int32(exp.OuterIndex),
		"positionX":         decodeIntLike(propMap["PositionX"]),
		"positionY":         decodeIntLike(propMap["PositionY"]),
		"settingsInterface": decodeObjectReference(propMap["SettingsInterface"]),
		"inputPins":         decodePCGObjectRefs(propMap["InputPins"]),
		"outputPins":        decodePCGObjectRefs(propMap["OutputPins"]),
	}
	if len(props.Warnings) > 0 {
		entry["warnings"] = append([]string(nil), props.Warnings...)
	}
	if includeProperties {
		entry["properties"] = toPropertyOutputs(asset, props.Properties, true)
	}
	return entry
}

func buildPCGPinEntry(asset *uasset.Asset, exportIndex int, includeProperties bool) map[string]any {
	exp := asset.Exports[exportIndex]
	props := asset.ParseExportProperties(exportIndex)
	propMap := decodedPropertyMap(asset, props.Properties)
	entry := map[string]any{
		"export":     exportIndex + 1,
		"objectName": exp.ObjectName.Display(asset.Names),
		"className":  asset.ResolveClassName(exp),
		"outerIndex": int32(exp.OuterIndex),
		"label":      decodeStringLike(propMap["Label"]),
		"direction":  decodeStringLike(propMap["Direction"]),
		"edges":      decodePCGObjectRefs(propMap["Edges"]),
	}
	if len(props.Warnings) > 0 {
		entry["warnings"] = append([]string(nil), props.Warnings...)
	}
	if includeProperties {
		entry["properties"] = toPropertyOutputs(asset, props.Properties, true)
	}
	return entry
}

func buildPCGSimpleEntry(asset *uasset.Asset, exportIndex int, includeProperties bool) map[string]any {
	exp := asset.Exports[exportIndex]
	entry := map[string]any{
		"export":     exportIndex + 1,
		"objectName": exp.ObjectName.Display(asset.Names),
		"className":  asset.ResolveClassName(exp),
		"outerIndex": int32(exp.OuterIndex),
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

func decodePCGObjectRefs(value any) []map[string]any {
	refs := decodeObjectReferenceArray(value)
	if refs == nil {
		return []map[string]any{}
	}
	return refs
}

func flattenPCGChildEntries(nodesRaw any, key string) []map[string]any {
	nodes, ok := nodesRaw.([]map[string]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(nodes)*2)
	for _, node := range nodes {
		children, ok := node[key].([]map[string]any)
		if !ok {
			continue
		}
		out = append(out, children...)
	}
	return out
}

func pcgGraphTargetExports(asset *uasset.Asset, exportFilter int) ([]int, error) {
	if asset == nil {
		return nil, fmt.Errorf("asset is nil")
	}
	if exportFilter > 0 {
		idx, err := asset.ResolveExportIndex(exportFilter)
		if err != nil {
			return nil, err
		}
		className := asset.ResolveClassName(asset.Exports[idx])
		if !isPCGGraphClassName(className) {
			return nil, fmt.Errorf("export %d is not a PCGGraph export (class=%s)", exportFilter, className)
		}
		return []int{idx}, nil
	}
	targets := make([]int, 0, 4)
	for i, exp := range asset.Exports {
		if isPCGGraphClassName(asset.ResolveClassName(exp)) {
			targets = append(targets, i)
		}
	}
	return targets, nil
}

func isPCGGraphClassName(className string) bool {
	return strings.EqualFold(strings.TrimSpace(className), "PCGGraph")
}

func isPCGNodeClassName(className string) bool {
	return strings.EqualFold(strings.TrimSpace(className), "PCGNode")
}

func isPCGSettingsClassName(className string) bool {
	low := strings.ToLower(strings.TrimSpace(className))
	return strings.HasPrefix(low, "pcg") && strings.HasSuffix(low, "settings")
}

func decodeIntLike(value any) int {
	switch t := value.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float64:
		return int(t)
	case map[string]any:
		if inner, ok := t["value"]; ok {
			return decodeIntLike(inner)
		}
	}
	return 0
}
