package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/wilddogjp/openbpx/pkg/uasset"
)

func runNiagara(args []string, stdout, stderr io.Writer) int {
	return dispatchSubcommand(
		args,
		stdout,
		stderr,
		"usage: bpx niagara <read> ...",
		"unknown niagara command: %s\n",
		subcommandSpec{Name: "read", Run: runNiagaraRead},
	)
}

func runNiagaraRead(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("niagara read", stderr)
	opts := registerCommonFlags(fs)
	exportIndex := fs.Int("export", 0, "optional 1-based NiagaraSystem export index")
	includeProperties := fs.Bool("include-properties", false, "include decoded property payloads for summarized Niagara exports")
	if err := parseFlagSet(fs, args); err != nil {
		return 1
	}
	if fs.NArg() != 1 || *exportIndex < 0 {
		fmt.Fprintln(stderr, "usage: bpx niagara read <file.uasset> [--export <n>] [--include-properties]")
		return 1
	}

	file := fs.Arg(0)
	asset, err := uasset.ParseFile(file, *opts)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	payload, err := buildNiagaraReadPayload(file, asset, *exportIndex, *includeProperties)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return printJSON(stdout, payload)
}

func buildNiagaraReadPayload(file string, asset *uasset.Asset, exportFilter int, includeProperties bool) (map[string]any, error) {
	if asset == nil {
		return nil, fmt.Errorf("asset is nil")
	}
	systemTargets, err := niagaraSystemTargetExports(asset, exportFilter)
	if err != nil {
		return nil, err
	}

	systemTargetSet := map[int]struct{}{}
	for _, idx := range systemTargets {
		systemTargetSet[idx] = struct{}{}
	}

	childCounts := childExportCountsByOuter(asset)
	systems := make([]map[string]any, 0, len(systemTargets))
	for _, idx := range systemTargets {
		systems = append(systems, buildNiagaraSystemEntry(asset, idx, includeProperties))
	}

	emitters := make([]map[string]any, 0, 16)
	graphs := make([]map[string]any, 0, 32)
	scripts := make([]map[string]any, 0, 32)
	renderers := make([]map[string]any, 0, 16)
	dataInterfaces := make([]map[string]any, 0, 32)
	scriptVariables := make([]map[string]any, 0, 32)

	for idx, exp := range asset.Exports {
		className := asset.ResolveClassName(exp)
		switch {
		case isNiagaraEmitterClassName(className):
			emitters = append(emitters, buildNiagaraEmitterEntry(asset, idx, includeProperties))
		case isNiagaraGraphClassName(className):
			graphs = append(graphs, buildNiagaraGraphEntry(asset, idx, childCounts[idx+1], className))
		case isNiagaraScriptClassName(className):
			scripts = append(scripts, buildNiagaraScriptEntry(asset, idx, includeProperties))
		case isNiagaraRendererClassName(className):
			renderers = append(renderers, buildNiagaraRendererEntry(asset, idx, includeProperties))
		case isNiagaraDataInterfaceClassName(className):
			dataInterfaces = append(dataInterfaces, buildNiagaraSimpleEntry(asset, idx, includeProperties))
		case isNiagaraScriptVariableClassName(className):
			scriptVariables = append(scriptVariables, buildNiagaraSimpleEntry(asset, idx, includeProperties))
		}
	}

	sortNiagaraEntries(emitters)
	sortNiagaraEntries(graphs)
	sortNiagaraEntries(scripts)
	sortNiagaraEntries(renderers)
	sortNiagaraEntries(dataInterfaces)
	sortNiagaraEntries(scriptVariables)

	resp := map[string]any{
		"file":                file,
		"exportFilter":        exportFilter,
		"includeProperties":   includeProperties,
		"systemCount":         len(systems),
		"emitterCount":        len(emitters),
		"graphCount":          len(graphs),
		"scriptCount":         len(scripts),
		"rendererCount":       len(renderers),
		"dataInterfaceCount":  len(dataInterfaces),
		"scriptVariableCount": len(scriptVariables),
		"systems":             systems,
		"emitters":            emitters,
		"graphs":              graphs,
		"scripts":             scripts,
		"renderers":           renderers,
		"dataInterfaces":      dataInterfaces,
		"scriptVariables":     scriptVariables,
		"mentalModel":         buildNiagaraMentalModel(systems, emitters, scripts, renderers, dataInterfaces, scriptVariables),
	}
	if len(systemTargetSet) == 0 {
		resp["note"] = "no NiagaraSystem exports found"
	}
	return resp, nil
}

func buildNiagaraMentalModel(systems, emitters, scripts, renderers, dataInterfaces, scriptVariables []map[string]any) map[string]any {
	out := map[string]any{
		"summary": "A Niagara package is typically composed of one or more NiagaraSystem exports that reference emitter handles, script exports, graphs, renderer properties, data interfaces, and script variables.",
		"systemLayer": []string{
			"NiagaraSystem holds emitter handles, scratch pad scripts, exposed parameters, and system-level spawn/update scripts.",
			"EmitterHandles point at versioned emitter exports and provide the top-level system composition.",
		},
		"emitterLayer": []string{
			"NiagaraEmitter exports hold versioned emitter data and unique emitter names.",
			"Renderers and data interfaces are usually sibling exports referenced by emitter/script data rather than embedded in one monolithic object.",
		},
		"scriptLayer": []string{
			"NiagaraScript exports capture usage plus VM data and rapid iteration parameter stores.",
			"NiagaraGraph and NiagaraScriptSource exports back the editable graph/source side of the system.",
		},
	}
	out["counts"] = map[string]any{
		"systems":         len(systems),
		"emitters":        len(emitters),
		"scripts":         len(scripts),
		"renderers":       len(renderers),
		"dataInterfaces":  len(dataInterfaces),
		"scriptVariables": len(scriptVariables),
	}
	return out
}

func buildNiagaraSystemEntry(asset *uasset.Asset, exportIndex int, includeProperties bool) map[string]any {
	exp := asset.Exports[exportIndex]
	props := asset.ParseExportProperties(exportIndex)
	propMap := decodedPropertyMap(asset, props.Properties)
	entry := map[string]any{
		"export":                           exportIndex + 1,
		"objectName":                       exp.ObjectName.Display(asset.Names),
		"className":                        asset.ResolveClassName(exp),
		"scratchPadScripts":                decodeObjectReferenceArray(propMap["ScratchPadScripts"]),
		"parameterDefinitionSubscriptions": decodeNiagaraParameterDefinitionSubscriptions(propMap["ParameterDefinitionsSubscriptions"]),
		"emitterHandles":                   decodeNiagaraEmitterHandles(propMap["EmitterHandles"]),
		"systemSpawnScript":                decodeObjectReference(propMap["SystemSpawnScript"]),
		"systemUpdateScript":               decodeObjectReference(propMap["SystemUpdateScript"]),
		"editorData":                       decodeObjectReference(propMap["EditorData"]),
		"editorParameters":                 decodeObjectReference(propMap["EditorParameters"]),
		"bakerSettings":                    decodeObjectReference(propMap["BakerSettings"]),
		"exposedParameters":                summarizeNiagaraParameterStore(propMap["ExposedParameters"]),
	}
	if len(props.Warnings) > 0 {
		entry["warnings"] = append([]string(nil), props.Warnings...)
	}
	if includeProperties {
		entry["properties"] = toPropertyOutputs(asset, props.Properties, true)
	}
	return entry
}

func buildNiagaraEmitterEntry(asset *uasset.Asset, exportIndex int, includeProperties bool) map[string]any {
	exp := asset.Exports[exportIndex]
	props := asset.ParseExportProperties(exportIndex)
	propMap := decodedPropertyMap(asset, props.Properties)
	entry := map[string]any{
		"export":            exportIndex + 1,
		"objectName":        exp.ObjectName.Display(asset.Names),
		"className":         asset.ResolveClassName(exp),
		"uniqueEmitterName": decodeStringLike(propMap["UniqueEmitterName"]),
		"versionDataCount":  decodeWrappedArrayCount(propMap["VersionData"]),
		"libraryVisibility": decodeStringLike(propMap["LibraryVisibility"]),
	}
	if len(props.Warnings) > 0 {
		entry["warnings"] = append([]string(nil), props.Warnings...)
	}
	if includeProperties {
		entry["properties"] = toPropertyOutputs(asset, props.Properties, true)
	}
	return entry
}

func buildNiagaraGraphEntry(asset *uasset.Asset, exportIndex int, childCount int, className string) map[string]any {
	exp := asset.Exports[exportIndex]
	return map[string]any{
		"export":       exportIndex + 1,
		"objectName":   exp.ObjectName.Display(asset.Names),
		"className":    className,
		"outerIndex":   int32(exp.OuterIndex),
		"childExports": childCount,
	}
}

func buildNiagaraScriptEntry(asset *uasset.Asset, exportIndex int, includeProperties bool) map[string]any {
	exp := asset.Exports[exportIndex]
	props := asset.ParseExportProperties(exportIndex)
	propMap := decodedPropertyMap(asset, props.Properties)
	entry := map[string]any{
		"export":                   exportIndex + 1,
		"objectName":               exp.ObjectName.Display(asset.Names),
		"className":                asset.ResolveClassName(exp),
		"usage":                    decodeStringLike(propMap["Usage"]),
		"versionDataCount":         decodeWrappedArrayCount(propMap["VersionData"]),
		"rapidIterationParameters": summarizeNiagaraParameterStore(propMap["RapidIterationParameters"]),
		"cachedScriptVMIdSummary":  summarizeStructuredValue(propMap["CachedScriptVMId"]),
		"cachedScriptVMSummary":    summarizeStructuredValue(propMap["CachedScriptVM"]),
	}
	if len(props.Warnings) > 0 {
		entry["warnings"] = append([]string(nil), props.Warnings...)
	}
	if includeProperties {
		entry["properties"] = toPropertyOutputs(asset, props.Properties, true)
	}
	return entry
}

func buildNiagaraRendererEntry(asset *uasset.Asset, exportIndex int, includeProperties bool) map[string]any {
	exp := asset.Exports[exportIndex]
	props := asset.ParseExportProperties(exportIndex)
	propMap := decodedPropertyMap(asset, props.Properties)
	entry := map[string]any{
		"export":     exportIndex + 1,
		"objectName": exp.ObjectName.Display(asset.Names),
		"className":  asset.ResolveClassName(exp),
		"material":   decodeObjectReference(propMap["Material"]),
		"bindings": map[string]any{
			"position":        summarizeStructuredValue(propMap["PositionBinding"]),
			"spriteSize":      summarizeStructuredValue(propMap["SpriteSizeBinding"]),
			"materialRandom":  summarizeStructuredValue(propMap["MaterialRandomBinding"]),
			"customSorting":   summarizeStructuredValue(propMap["CustomSortingBinding"]),
			"normalizedAge":   summarizeStructuredValue(propMap["NormalizedAgeBinding"]),
			"rendererEnabled": summarizeStructuredValue(propMap["RendererEnabledBinding"]),
		},
	}
	if len(props.Warnings) > 0 {
		entry["warnings"] = append([]string(nil), props.Warnings...)
	}
	if includeProperties {
		entry["properties"] = toPropertyOutputs(asset, props.Properties, true)
	}
	return entry
}

func buildNiagaraSimpleEntry(asset *uasset.Asset, exportIndex int, includeProperties bool) map[string]any {
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

func decodedPropertyMap(asset *uasset.Asset, props []uasset.PropertyTag) map[string]any {
	out := make(map[string]any, len(props))
	for _, p := range props {
		if decoded, ok := asset.DecodePropertyValue(p); ok {
			out[p.Name.Display(asset.Names)] = decoded
		}
	}
	return out
}

func decodeObjectReference(value any) map[string]any {
	m, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	if inner, ok := m["value"]; ok {
		if ref := decodeObjectReference(inner); ref != nil {
			return ref
		}
	}
	out := map[string]any{}
	if resolved, _ := m["resolved"].(string); strings.TrimSpace(resolved) != "" {
		out["resolved"] = resolved
	}
	if index, ok := m["index"]; ok {
		out["index"] = index
	}
	if assetName, _ := m["assetName"].(string); assetName != "" {
		out["assetName"] = assetName
	}
	if packageName, _ := m["packageName"].(string); packageName != "" {
		out["packageName"] = packageName
	}
	if subPath, _ := m["subPath"].(string); subPath != "" {
		out["subPath"] = subPath
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func decodeObjectReferenceArray(value any) []map[string]any {
	root, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	rawItems, ok := asNiagaraAnySlice(root["value"])
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(rawItems))
	for _, rawItem := range rawItems {
		if wrapper, ok := rawItem.(map[string]any); ok {
			if inner, ok := wrapper["value"]; ok {
				if ref := decodeObjectReference(inner); ref != nil {
					out = append(out, ref)
				}
			}
		}
	}
	return out
}

func decodeNiagaraParameterDefinitionSubscriptions(value any) []map[string]any {
	items := decodeWrappedStructArray(value)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry := map[string]any{
			"definitions": decodeObjectReference(item["Definitions"]),
		}
		if hash, ok := item["CachedChangeIdHash"].(map[string]any); ok {
			if inner, exists := hash["value"]; exists {
				entry["cachedChangeIdHash"] = inner
			}
		}
		out = append(out, entry)
	}
	return out
}

func decodeNiagaraEmitterHandles(value any) []map[string]any {
	items := decodeWrappedStructArray(value)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry := map[string]any{
			"name":        decodeNameLike(item["Name"]),
			"id":          decodeStringLike(item["Id"]),
			"idName":      decodeNameLike(item["IdName"]),
			"emitterMode": decodeStringLike(item["EmitterMode"]),
			"isEnabled":   decodeBoolLike(item["bIsEnabled"]),
		}
		if versionedFields, ok := unwrapStructValue(item["VersionedInstance"]); ok {
			entry["emitter"] = decodeObjectReference(versionedFields["Emitter"])
			entry["version"] = decodeStringLike(versionedFields["Version"])
		}
		out = append(out, entry)
	}
	return out
}

func summarizeNiagaraParameterStore(value any) map[string]any {
	structValue, ok := unwrapStructValue(value)
	if !ok {
		return nil
	}
	out := map[string]any{}
	if uobjects := decodeObjectReferenceArray(structValue["UObjects"]); len(uobjects) > 0 {
		out["uobjects"] = uobjects
		out["uobjectCount"] = len(uobjects)
	}
	if redirs := summarizeWrappedMap(structValue["UserParameterRedirects"]); redirs != nil {
		out["userParameterRedirects"] = redirs
	}
	if paramOffsets := summarizeWrappedArray(structValue["SortedParameterOffsets"]); paramOffsets != nil {
		out["sortedParameterOffsets"] = paramOffsets
	}
	if parameterData := summarizeWrappedArray(structValue["ParameterData"]); parameterData != nil {
		out["parameterData"] = parameterData
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func decodeWrappedArrayCount(value any) int {
	root, ok := value.(map[string]any)
	if !ok {
		return 0
	}
	rawItems, ok := asNiagaraAnySlice(root["value"])
	if !ok {
		return 0
	}
	return len(rawItems)
}

func summarizeStructuredValue(value any) map[string]any {
	structType, fields, ok := materialStructFields(value)
	if !ok {
		return nil
	}
	out := map[string]any{}
	if structType != "" {
		out["structType"] = structType
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out["fields"] = keys
	return out
}

func summarizeWrappedArray(value any) map[string]any {
	root, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	if arrayType, _ := root["arrayType"].(string); arrayType != "" {
		out["arrayType"] = arrayType
	}
	if rawItems, ok := asNiagaraAnySlice(root["value"]); ok {
		out["count"] = len(rawItems)
	}
	if rawBase64, _ := root["rawBase64"].(string); rawBase64 != "" {
		out["rawBase64"] = true
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func summarizeWrappedMap(value any) map[string]any {
	root, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	if keyType, _ := root["keyType"].(string); keyType != "" {
		out["keyType"] = keyType
	}
	if valueType, _ := root["valueType"].(string); valueType != "" {
		out["valueType"] = valueType
	}
	if rawItems, ok := asNiagaraAnySlice(root["value"]); ok {
		out["count"] = len(rawItems)
	}
	if rawBase64, _ := root["rawBase64"].(string); rawBase64 != "" {
		out["rawBase64"] = true
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func decodeWrappedStructArray(value any) []map[string]any {
	root, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	rawItems, ok := asAnySlice(root["value"])
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(rawItems))
	for _, rawItem := range rawItems {
		wrapper, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		if itemValue, ok := wrapper["value"].(map[string]any); ok {
			if fields, ok := itemValue["value"].(map[string]any); ok {
				out = append(out, fields)
			}
		}
	}
	return out
}

func unwrapStructValue(value any) (map[string]any, bool) {
	root, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	fields, ok := root["value"].(map[string]any)
	if !ok {
		return nil, false
	}
	if _, hasStructType := fields["structType"]; hasStructType {
		if innerFields, ok := fields["value"].(map[string]any); ok {
			return innerFields, true
		}
	}
	return fields, true
}

func decodeNameLike(value any) string {
	switch t := value.(type) {
	case map[string]any:
		if name, _ := t["name"].(string); name != "" {
			return name
		}
		if inner, ok := t["value"]; ok {
			return decodeNameLike(inner)
		}
	case string:
		return t
	}
	return ""
}

func decodeStringLike(value any) string {
	switch t := value.(type) {
	case string:
		return t
	case map[string]any:
		if inner, ok := t["value"]; ok {
			return decodeStringLike(inner)
		}
		if name, _ := t["name"].(string); name != "" {
			return name
		}
		if structType, _ := t["structType"].(string); structType != "" {
			if inner, ok := t["value"]; ok {
				return decodeStringLike(inner)
			}
		}
	}
	return ""
}

func decodeBoolLike(value any) bool {
	if b, ok := value.(bool); ok {
		return b
	}
	if m, ok := value.(map[string]any); ok {
		if inner, ok := m["value"]; ok {
			return decodeBoolLike(inner)
		}
	}
	return false
}

func asNiagaraAnySlice(value any) ([]any, bool) {
	switch t := value.(type) {
	case []any:
		return t, true
	case []map[string]any:
		out := make([]any, 0, len(t))
		for _, item := range t {
			out = append(out, item)
		}
		return out, true
	default:
		return nil, false
	}
}

func niagaraSystemTargetExports(asset *uasset.Asset, exportFilter int) ([]int, error) {
	if asset == nil {
		return nil, fmt.Errorf("asset is nil")
	}
	if exportFilter > 0 {
		idx, err := asset.ResolveExportIndex(exportFilter)
		if err != nil {
			return nil, err
		}
		className := asset.ResolveClassName(asset.Exports[idx])
		if !isNiagaraSystemClassName(className) {
			return nil, fmt.Errorf("export %d is not a NiagaraSystem export (class=%s)", exportFilter, className)
		}
		return []int{idx}, nil
	}
	targets := make([]int, 0, 4)
	for i, exp := range asset.Exports {
		if isNiagaraSystemClassName(asset.ResolveClassName(exp)) {
			targets = append(targets, i)
		}
	}
	return targets, nil
}

func childExportCountsByOuter(asset *uasset.Asset) map[int]int {
	out := map[int]int{}
	if asset == nil {
		return out
	}
	for _, exp := range asset.Exports {
		if exp.OuterIndex > 0 {
			out[int(exp.OuterIndex)]++
		}
	}
	return out
}

func sortNiagaraEntries(items []map[string]any) {
	sort.Slice(items, func(i, j int) bool {
		leftExport, _ := items[i]["export"].(int)
		rightExport, _ := items[j]["export"].(int)
		if leftExport != rightExport {
			return leftExport < rightExport
		}
		leftName, _ := items[i]["objectName"].(string)
		rightName, _ := items[j]["objectName"].(string)
		return leftName < rightName
	})
}

func isNiagaraSystemClassName(className string) bool {
	return strings.EqualFold(strings.TrimSpace(className), "NiagaraSystem")
}

func isNiagaraEmitterClassName(className string) bool {
	return strings.EqualFold(strings.TrimSpace(className), "NiagaraEmitter")
}

func isNiagaraGraphClassName(className string) bool {
	return strings.EqualFold(strings.TrimSpace(className), "NiagaraGraph")
}

func isNiagaraScriptClassName(className string) bool {
	return strings.EqualFold(strings.TrimSpace(className), "NiagaraScript")
}

func isNiagaraRendererClassName(className string) bool {
	low := strings.ToLower(strings.TrimSpace(className))
	return strings.Contains(low, "niagara") && strings.Contains(low, "rendererproperties")
}

func isNiagaraDataInterfaceClassName(className string) bool {
	return strings.HasPrefix(strings.TrimSpace(className), "NiagaraDataInterface")
}

func isNiagaraScriptVariableClassName(className string) bool {
	return strings.EqualFold(strings.TrimSpace(className), "NiagaraScriptVariable")
}
