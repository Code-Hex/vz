package main

import (
	"bytes"
	"debug/macho"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type swiftTargetInfo struct {
	Target struct {
		CompatibilityLibraries []struct {
			LibraryName string
			Filter      string
			ForceLoad   *bool
		}
	}
	Paths struct {
		RuntimeLibraryPaths []string
	}
}

func compileSwiftObjects(arguments, sources []string, header, directory string) ([]string, error) {
	outputs := make(map[string]map[string]string, len(sources))
	objects := make([]string, 0, len(sources))
	for _, source := range sources {
		object := strings.TrimSuffix(source, ".swift") + ".o"
		outputs[source] = map[string]string{"object": object}
		objects = append(objects, object)
	}
	data, err := json.Marshal(outputs)
	if err != nil {
		return nil, err
	}
	outputMap := filepath.Join(directory, "outputs.json")
	if err := os.WriteFile(outputMap, data, 0600); err != nil {
		return nil, err
	}
	filelist := filepath.Join(directory, "sources.rsp")
	var quoted []string
	for _, source := range sources {
		quoted = append(quoted, strconv.Quote(source))
	}
	if err := os.WriteFile(filelist, []byte(strings.Join(quoted, "\n")+"\n"), 0600); err != nil {
		return nil, err
	}
	args := append(append([]string(nil), arguments...), "-c", "-enable-batch-mode", "-driver-batch-count", "1", "-emit-objc-header", "-emit-objc-header-path", header, "-output-file-map", outputMap, "-driver-force-response-files", "@"+filelist)
	if _, err := command("xcrun", args...); err != nil {
		return nil, err
	}
	return objects, nil
}

func checkObjectExports(objects, symbols []string) (string, error) {
	remaining := make(map[string]bool, len(symbols))
	for _, symbol := range symbols {
		remaining["_"+symbol] = true
	}
	var abiObject string
	for _, object := range objects {
		file, err := macho.Open(object)
		if err != nil {
			return "", err
		}
		if file.Symtab == nil {
			file.Close()
			return "", fmt.Errorf("Swift object has no symbol table: %s", object)
		}
		var exports []string
		for _, symbol := range file.Symtab.Syms {
			if symbol.Type == 0xf && strings.HasPrefix(symbol.Name, "_vz_") {
				exports = append(exports, symbol.Name)
			}
		}
		file.Close()
		if len(exports) > 1 {
			return "", fmt.Errorf("Swift object %s contains multiple bridge exports: %v", object, exports)
		}
		for _, symbol := range exports {
			if !remaining[symbol] {
				return "", fmt.Errorf("unexpected or duplicate bridge export %s in %s", symbol, object)
			}
			delete(remaining, symbol)
			if symbol == "_vz_bridge_abi" {
				abiObject = object
			}
		}
	}
	if len(remaining) != 0 {
		var missing []string
		for symbol := range remaining {
			missing = append(missing, symbol)
		}
		sort.Strings(missing)
		return "", fmt.Errorf("missing Swift object exports: %v", missing)
	}
	return abiObject, nil
}

func bundleStaticLibrary(objects []string, abiObject, output, target, arch, sdkPath string, sdkVersion int) error {
	data, err := command("xcrun", "swiftc", "-print-target-info", "-target", target)
	if err != nil {
		return err
	}
	var info swiftTargetInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return fmt.Errorf("Swift target information: %w", err)
	}
	sdk := fmt.Sprintf("%d.%d.%d", sdkVersion/10000, sdkVersion/100%100, sdkVersion%100)
	object := abiObject + ".compat.o"
	args := []string{"ld", "-r", "-arch", arch, "-platform_version", "macos", "11.0", sdk, abiObject}
	var optionalLibraries []string
	for _, dependency := range info.Target.CompatibilityLibraries {
		if dependency.Filter != "all" {
			return fmt.Errorf("unsupported Swift compatibility library filter %q for %s", dependency.Filter, dependency.LibraryName)
		}
		var library string
		for _, directory := range info.Paths.RuntimeLibraryPaths {
			candidate := filepath.Join(directory, "lib"+dependency.LibraryName+".a")
			stat, err := os.Stat(candidate)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			if stat.Mode().IsRegular() {
				library = candidate
				break
			}
		}
		if library == "" {
			return fmt.Errorf("Swift compatibility library %s not found in %v", dependency.LibraryName, info.Paths.RuntimeLibraryPaths)
		}
		if dependency.ForceLoad == nil || *dependency.ForceLoad {
			args = append(args, "-force_load", library)
		} else {
			optionalLibraries = append(optionalLibraries, library)
		}
	}
	args = append(args, "-o", object)
	if _, err := command("xcrun", args...); err != nil {
		return err
	}
	var members []string
	for _, member := range objects {
		if member != abiObject {
			members = append(members, member)
		}
	}
	members = append(members, object)
	members = append(members, optionalLibraries...)
	filelist := output + ".filelist"
	if err := os.WriteFile(filelist, []byte(strings.Join(members, "\n")+"\n"), 0600); err != nil {
		return err
	}
	if _, err := command("xcrun", "libtool", "-static", "-arch_only", arch, "-no_warning_for_no_symbols", "-filelist", filelist, "-o", output); err != nil {
		return err
	}
	probe := output + ".dylib"
	if _, err := command("xcrun", "clang", "-dynamiclib", "-target", target, "-isysroot", sdkPath, "-Xlinker", "-force_load", "-Xlinker", output, "-L/usr/lib/swift", "-framework", "Foundation", "-framework", "Virtualization", "-framework", "Cocoa", "-o", probe); err != nil {
		return err
	}
	return writeFrameworkStub(probe, filepath.Join(filepath.Dir(output), "Virtualization.tbd"), arch)
}

func writeFrameworkStub(probe, output, arch string) error {
	file, err := macho.Open(probe)
	if err != nil {
		return err
	}
	defer file.Close()
	const (
		framework         = "/System/Library/Frameworks/Virtualization.framework/Versions/A/Virtualization"
		loadDylib         = 0xc
		loadWeakDylib     = 0x80000018
		reexportDylib     = 0x8000001f
		lazyLoadDylib     = 0x20
		loadUpwardDylib   = 0x80000023
		undefinedExternal = 0x1
		weakReference     = 0x40
	)
	// ImportedLibraries omits weak dylibs, which still occupy library ordinals.
	ordinal, frameworkOrdinal := 0, -1
	for _, load := range file.Loads {
		raw := load.Raw()
		switch file.ByteOrder.Uint32(raw) {
		case loadDylib, loadWeakDylib, reexportDylib, lazyLoadDylib, loadUpwardDylib:
			ordinal++
		default:
			continue
		}
		if len(raw) < 24 {
			return fmt.Errorf("invalid dylib load command in %s", probe)
		}
		offset := file.ByteOrder.Uint32(raw[8:])
		if offset >= uint32(len(raw)) {
			return fmt.Errorf("invalid dylib name offset in %s", probe)
		}
		name := raw[offset:]
		if end := bytes.IndexByte(name, 0); end >= 0 {
			name = name[:end]
		}
		if string(name) == framework {
			frameworkOrdinal = ordinal
		}
	}
	var symbols []string
	if file.Symtab == nil {
		return fmt.Errorf("native bridge probe has no symbol table: %s", probe)
	}
	for _, symbol := range file.Symtab.Syms {
		if symbol.Type == undefinedExternal && symbol.Desc&weakReference != 0 && int(symbol.Desc>>8) == frameworkOrdinal {
			symbols = append(symbols, symbol.Name)
		}
	}
	sort.Strings(symbols)
	var stub strings.Builder
	fmt.Fprintf(&stub, "--- !tapi-tbd\ntbd-version: 4\ntargets: [ %s-macos ]\ninstall-name: %q\n", arch, framework)
	if len(symbols) == 0 {
		stub.WriteString("exports: []\n")
	} else {
		fmt.Fprintf(&stub, "exports:\n  - targets: [ %s-macos ]\n    symbols:\n", arch)
		for _, symbol := range symbols {
			fmt.Fprintf(&stub, "      - %q\n", symbol)
		}
	}
	stub.WriteString("...\n")
	return os.WriteFile(output, []byte(stub.String()), 0644)
}
