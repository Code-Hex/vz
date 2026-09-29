// Command vzbridgegen compiles the native Swift bridge and generates its purego ABI.
package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

type inputs struct {
	Swift, ObjC []string
	Hash        string
	Contents    map[string][]byte
}

func main() {
	source := flag.String("source", "internal/vzbridge/source", "handwritten native support sources")
	output := flag.String("output", "internal/vzbridge", "generated bridge directory")
	list := flag.Bool("list", false, "list public SDK declarations")
	listPrivate := flag.Bool("list-private", false, "list Objective-C runtime methods in Virtualization.framework")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "vzbridgegen: unexpected arguments")
		os.Exit(1)
	}
	var err error
	if *list || *listPrivate {
		err = listFramework(*list, *listPrivate)
	} else {
		err = build(*source, *output, true)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "vzbridgegen:", err)
		os.Exit(1)
	}
}

func command(name string, args ...string) ([]byte, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s %q: %w\n%s", name, args, err, out)
	}
	if strings.Contains(string(out), ": warning:") {
		return nil, fmt.Errorf("%s emitted a compiler warning: %s", name, out)
	}
	return out, nil
}

func readInputs(source string) (inputs, error) {
	in := inputs{Contents: make(map[string][]byte)}
	var files []string
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return in, err
	}
	sort.Strings(files)
	hash := sha256.New()
	for _, path := range files {
		switch filepath.Ext(path) {
		case ".swift":
			in.Swift = append(in.Swift, path)
		case ".m":
			in.ObjC = append(in.ObjC, path)
		case ".h":
		default:
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return in, err
		}
		relative, _ := filepath.Rel(source, path)
		in.Contents[relative] = data
		fmt.Fprintf(hash, "%s\x00%d\x00", relative, len(data))
		hash.Write(data)
	}
	if len(in.Swift) == 0 {
		return in, fmt.Errorf("source requires Swift implementation")
	}
	in.Hash = fmt.Sprintf("%x", hash.Sum(nil))
	return in, nil
}

func build(source, output string, framework bool) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("generation requires macOS")
	}
	source, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return err
	}
	in, err := readInputs(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(output, 0755); err != nil {
		return err
	}
	temp, err := os.MkdirTemp(output, ".vzbridgegen-")
	if err != nil {
		return err
	}
	keepBuild := false
	defer func() {
		if !keepBuild {
			os.RemoveAll(temp)
		}
	}()
	snapshot := filepath.Join(temp, "source")
	for name, data := range in.Contents {
		path := filepath.Join(snapshot, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return err
		}
	}
	source = snapshot
	in, err = readInputs(source)
	if err != nil {
		return err
	}
	sdkData, err := command("xcrun", "--sdk", "macosx", "--show-sdk-path")
	if err != nil {
		return err
	}
	sdk := strings.TrimSpace(string(sdkData))
	sdkVersionData, err := command("xcrun", "--sdk", "macosx", "--show-sdk-version")
	if err != nil {
		return err
	}
	sdkVersion, err := sdkVersionConstant(strings.TrimSpace(string(sdkVersionData)))
	if err != nil {
		return err
	}
	var private privateMetadata
	if framework {
		private, err = discoverPrivate(temp, nil)
		if err != nil {
			return err
		}
		if private.Architecture != runtime.GOARCH {
			return fmt.Errorf("private runtime architecture %q differs from generator host %s", private.Architecture, runtime.GOARCH)
		}
	}
	for _, arch := range []string{"arm64", "amd64"} {
		if err := buildArchitecture(in, source, temp, sdk, sdkVersion, arch, framework, private); err != nil {
			return err
		}
	}
	if err := publishBuild(temp, output); err != nil {
		keepBuild = true
		return fmt.Errorf("%w; publication artifacts retained at %s", err, temp)
	}
	return nil
}

func buildArchitecture(in inputs, source, temp, sdk string, sdkVersion int, arch string, framework bool, private privateMetadata) error {
	dir := filepath.Join(temp, "abi_"+arch)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	targetArch := arch
	if arch == "amd64" {
		targetArch = "x86_64"
	}
	target := targetArch + "-apple-macosx11.0"
	cache := filepath.Join(temp, "cache_"+arch)
	astArgs := []string{"swiftc", "-dump-ast", "-swift-version", "6", "-strict-concurrency=complete", "-warnings-as-errors", "-parse-as-library", "-sdk", sdk, "-target", target, "-module-cache-path", cache, "-I", source}
	if _, err := os.Stat(filepath.Join(source, "BridgeSupport.h")); err == nil {
		astArgs = append(astArgs, "-import-objc-header", filepath.Join(source, "BridgeSupport.h"))
	}
	astArgs = append(astArgs, in.Swift...)
	swiftAST, err := command("xcrun", astArgs...)
	if err != nil {
		return err
	}
	contracts, err := inferOperations(strings.NewReader(string(swiftAST)))
	if err != nil {
		return err
	}
	var generated strings.Builder
	generated.WriteString("// Code generated by vzbridgegen; DO NOT EDIT.\n")
	if framework {
		symbols, err := discoverFramework(sdk, target, temp)
		if err != nil {
			return err
		}
		code, operations, skipped, err := generateAllFramework(symbols)
		if err != nil {
			return err
		}
		generated.WriteString(code)
		contracts = append(contracts, operations...)
		fmt.Fprintf(os.Stderr, "%s: generated %d SDK bindings; %d declarations skipped\n", arch, len(operations), len(skipped))
		for _, reason := range skipped {
			fmt.Fprintf(os.Stderr, "%s SDK: %s\n", arch, reason)
		}
		privateCode, privateOperations, skippedPrivate, err := generatePrivate(private)
		if err != nil {
			return err
		}
		generated.Write(privateCode)
		contracts = append(contracts, privateOperations...)
		fmt.Fprintf(os.Stderr, "%s: generated %d runtime bindings from %s host runtime %s; %d methods skipped\n", arch, len(privateOperations), private.Architecture, private.OSVersion, len(skippedPrivate))
		for _, reason := range skippedPrivate {
			fmt.Fprintf(os.Stderr, "%s runtime: %s\n", arch, reason)
		}
	}
	generatedPath := filepath.Join(dir, "Framework.swift")
	if err := os.WriteFile(generatedPath, []byte(generated.String()), 0644); err != nil {
		return err
	}
	in.Swift = append(append([]string(nil), in.Swift...), generatedPath)
	sort.Slice(contracts, func(i, j int) bool { return contracts[i].Symbol < contracts[j].Symbol })

	var objects []string
	for i, path := range in.ObjC {
		object := filepath.Join(temp, fmt.Sprintf("%s_%d.o", arch, i))
		args := []string{"clang", "-target", target, "-isysroot", sdk, "-fno-objc-arc", "-Werror", "-I", source, "-c", path, "-o", object}
		if _, err := command("xcrun", args...); err != nil {
			return err
		}
		objects = append(objects, object)
	}
	header := filepath.Join(temp, "Bridge_"+arch+".h")
	library := filepath.Join(temp, "Swift_"+arch+".a")
	swiftArgs := []string{"swiftc", "-swift-version", "6", "-strict-concurrency=complete", "-warnings-as-errors", "-diagnostic-style", "llvm", "-no-color-diagnostics", "-parse-as-library", "-disable-autolinking-runtime-compatibility", "-disable-autolinking-runtime-compatibility-concurrency", "-module-name", fmt.Sprintf("VZNativeBridge_%s_%d", in.Hash[:16], sdkVersion), "-sdk", sdk, "-target", target, "-module-cache-path", cache, "-I", source}
	// The bridge does not use Darwin's float constants. Their overlay's force-load symbol requires macOS 15.
	swiftArgs = append(swiftArgs, "-Xfrontend", "-disable-autolink-library", "-Xfrontend", "swift_Builtin_float")
	if _, err := os.Stat(filepath.Join(source, "BridgeSupport.h")); err == nil {
		swiftArgs = append(swiftArgs, "-import-objc-header", filepath.Join(source, "BridgeSupport.h"))
	}
	args := append(append([]string(nil), swiftArgs...), "-emit-library", "-static", "-emit-objc-header", "-emit-objc-header-path", header, "-o", library)
	args = append(args, in.Swift...)
	args = append(args, objects...)
	if _, err := command("xcrun", args...); err != nil {
		return err
	}
	ast, err := command("xcrun", "clang", "-target", target, "-isysroot", sdk, "-fsyntax-only", "-x", "objective-c", "-Xclang", "-ast-dump=json", "-Xclang", "-ast-dump-filter=vz_", "-include", "Virtualization/Virtualization.h", header)
	if err != nil {
		return err
	}
	calls, err := checkHeader(strings.NewReader(string(ast)), contracts, arch)
	if err != nil {
		return err
	}
	release := false
	for _, call := range calls {
		release = release || call.Name == "ReleaseObject" && call.Result.Go == "" && len(call.Parameters) == 1 && call.Parameters[0].Role == "raw"
	}
	if !release {
		return fmt.Errorf("ReleaseObject(raw pointer) contract is required")
	}
	encoded, _ := json.Marshal(struct {
		Schema                    int
		SDKVersion                int
		Target, Source, Generated string
		Calls                     []checkedOperation
	}{2, sdkVersion, target, in.Hash, generated.String(), calls})
	sum := sha256.Sum256(encoded)
	var hash [4]uint64
	for i := range hash {
		hash[i] = binary.LittleEndian.Uint64(sum[i*8:])
	}
	abiSource := filepath.Join(temp, "ABI_"+arch+".swift")
	var code strings.Builder
	code.WriteString("@c(vz_bridge_abi)\npublic func bridgeABI(_ index: UInt32) -> UInt64 {\n switch index {\n")
	for i, word := range hash {
		fmt.Fprintf(&code, " case %d: return %d\n", i, word)
	}
	code.WriteString(" default: return 0\n }\n}\n")
	if err := os.WriteFile(abiSource, []byte(code.String()), 0644); err != nil {
		return err
	}
	symbols := []string{"vz_bridge_abi"}
	for _, call := range calls {
		symbols = append(symbols, call.Symbol)
	}
	splitDirectory := filepath.Join(temp, "split_"+arch)
	splitSources, err := splitSwiftSources(append(in.Swift, abiSource), symbols, splitDirectory)
	if err != nil {
		return err
	}
	splitHeader := filepath.Join(splitDirectory, "Bridge.h")
	swiftObjects, err := compileSwiftObjects(swiftArgs, splitSources, splitHeader, splitDirectory)
	if err != nil {
		return err
	}
	ast, err = command("xcrun", "clang", "-target", target, "-isysroot", sdk, "-fsyntax-only", "-x", "objective-c", "-Xclang", "-ast-dump=json", "-Xclang", "-ast-dump-filter=vz_", "-include", "Virtualization/Virtualization.h", splitHeader)
	if err != nil {
		return err
	}
	splitCalls, err := checkHeader(strings.NewReader(string(ast)), contracts, arch)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(calls, splitCalls) {
		return fmt.Errorf("split Swift sources changed the bridge ABI for %s", arch)
	}
	abiObject, err := checkObjectExports(swiftObjects, symbols)
	if err != nil {
		return err
	}
	objects = append(objects, swiftObjects...)
	if err := bundleStaticLibrary(objects, abiObject, filepath.Join(dir, "libBridge.a"), target, targetArch, sdk, sdkVersion); err != nil {
		return err
	}
	goCode, err := bindings(calls, hash, arch, sdkVersion)
	if err != nil {
		return err
	}
	linkHash := sha256.New()
	for _, name := range []string{"libBridge.a", "Virtualization.tbd"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		fmt.Fprintf(linkHash, "%s\x00%d\x00", name, len(data))
		linkHash.Write(data)
	}
	goCode = []byte(strings.Replace(string(goCode), "\n", fmt.Sprintf("\n// Native link inputs SHA-256: %x\n", linkHash.Sum(nil)), 1))
	return os.WriteFile(filepath.Join(temp, "binding_"+arch+".go"), goCode, 0644)
}

func sdkVersionConstant(version string) (int, error) {
	parts := strings.Split(version, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return 0, fmt.Errorf("invalid SDK version %q", version)
	}
	values := [3]int{}
	for i, part := range parts {
		v, err := strconv.Atoi(part)
		if err != nil || v < 0 || v >= 100 {
			return 0, fmt.Errorf("invalid SDK version %q", version)
		}
		values[i] = v
	}
	return values[0]*10000 + values[1]*100 + values[2], nil
}
