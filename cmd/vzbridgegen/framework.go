package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type frameworkFragment struct {
	Kind              string `json:"kind"`
	Spelling          string `json:"spelling"`
	PreciseIdentifier string `json:"preciseIdentifier"`
}

type frameworkVersion struct{ Major, Minor, Patch int }

type frameworkAvailability struct {
	Domain      string            `json:"domain"`
	Introduced  *frameworkVersion `json:"introduced"`
	Obsoleted   *frameworkVersion `json:"obsoleted"`
	Unavailable bool              `json:"isUnconditionallyUnavailable"`
}

type frameworkSymbol struct {
	Kind struct {
		Identifier string `json:"identifier"`
	} `json:"kind"`
	Identifier struct {
		Precise string `json:"precise"`
	} `json:"identifier"`
	Path         []string                `json:"pathComponents"`
	Fragments    []frameworkFragment     `json:"declarationFragments"`
	Availability []frameworkAvailability `json:"availability"`
	Signature    struct {
		Parameters []struct {
			Name      string              `json:"name"`
			Fragments []frameworkFragment `json:"declarationFragments"`
		} `json:"parameters"`
		Returns []frameworkFragment `json:"returns"`
	} `json:"functionSignature"`
	EnumABI            string
	ExternalSuperclass string
}

func discoverFramework(sdk, target, temp string) ([]frameworkSymbol, error) {
	dir, err := os.MkdirTemp(temp, "framework-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	_, err = command("xcrun", "swift-symbolgraph-extract", "-module-name", "Virtualization", "-target", target, "-sdk", sdk, "-output-dir", dir, "-skip-synthesized-members", "-skip-inherited-docs", "-module-cache-path", filepath.Join(dir, "cache"))
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "Virtualization.symbols.json"))
	if err != nil {
		return nil, err
	}
	var graph struct {
		Symbols       []frameworkSymbol                                       `json:"symbols"`
		Relationships []struct{ Kind, Source, Target, TargetFallback string } `json:"relationships"`
	}
	if err := json.Unmarshal(data, &graph); err != nil {
		return nil, err
	}
	ast, err := command("xcrun", "clang", "-target", target, "-isysroot", sdk, "-x", "objective-c", "-fsyntax-only", "-Xclang", "-ast-dump=json", "-Xclang", "-ast-dump-filter=VZ", "-include", "Virtualization/Virtualization.h", "/dev/null")
	if err != nil {
		return nil, err
	}
	enums := map[string]string{}
	decoder := json.NewDecoder(strings.NewReader(string(ast)))
	for {
		var n struct {
			Kind, Name          string
			FixedUnderlyingType struct{ QualType, DesugaredQualType string }
		}
		err := decoder.Decode(&n)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if n.Kind != "EnumDecl" {
			continue
		}
		underlying := n.FixedUnderlyingType.DesugaredQualType
		if underlying == "" {
			underlying = n.FixedUnderlyingType.QualType
		}
		switch underlying {
		case "long", "long long":
			enums[n.Name] = "Int64"
		case "unsigned long", "unsigned long long":
			enums[n.Name] = "UInt64"
		case "int":
			enums[n.Name] = "Int32"
		case "unsigned int":
			enums[n.Name] = "UInt32"
		}
	}
	externalSuperclasses := map[string]string{}
	for _, relationship := range graph.Relationships {
		if relationship.Kind == "inheritsFrom" && relationship.TargetFallback != "" && relationship.Target != "c:objc(cs)NSObject" {
			externalSuperclasses[relationship.Source] = relationship.TargetFallback
		}
	}
	for i := range graph.Symbols {
		s := &graph.Symbols[i]
		s.ExternalSuperclass = externalSuperclasses[s.Identifier.Precise]
		if s.Kind.Identifier == "swift.enum" && strings.HasPrefix(s.Identifier.Precise, "c:@E@") {
			s.EnumABI = enums[strings.TrimPrefix(s.Identifier.Precise, "c:@E@")]
		}
	}
	return graph.Symbols, nil
}

func frameworkSpelling(fragments []frameworkFragment) string {
	var b strings.Builder
	for _, f := range fragments {
		b.WriteString(f.Spelling)
	}
	return b.String()
}

func frameworkTypeFragments(fragments []frameworkFragment) []frameworkFragment {
	for i, f := range fragments {
		if f.Kind == "text" && strings.Contains(f.Spelling, ": ") {
			result := append([]frameworkFragment{{Kind: "text", Spelling: strings.SplitN(f.Spelling, ": ", 2)[1]}}, fragments[i+1:]...)
			for j, x := range result {
				if x.Kind == "text" && strings.Contains(x.Spelling, " {") {
					result[j].Spelling = strings.SplitN(x.Spelling, " {", 2)[0]
					result = result[:j+1]
					break
				}
			}
			return result
		}
	}
	return nil
}

type frameworkType struct {
	Swift, ABI, Role, EnumABI string
	Optional                  bool
}

func frameworkValueType(fragments []frameworkFragment, symbols map[string]frameworkSymbol, result bool) (frameworkType, error) {
	spelling := strings.TrimSpace(frameworkSpelling(fragments))
	t := frameworkType{Swift: spelling, Optional: strings.HasSuffix(spelling, "?")}
	base := strings.TrimSuffix(spelling, "?")
	if base == "" || base == "()" || base == "Void" {
		return frameworkType{Swift: "Void"}, nil
	}
	scalar := map[string]string{"Bool": "Bool", "Int": "Int64", "UInt": "UInt64", "Int8": "Int8", "UInt8": "UInt8", "Int16": "Int16", "UInt16": "UInt16", "Int32": "Int32", "UInt32": "UInt32", "Int64": "Int64", "UInt64": "UInt64", "Float": "Float", "Double": "Double"}
	if abi, ok := scalar[base]; ok && !t.Optional {
		t.ABI = abi
		return t, nil
	}
	if base == "String" {
		if t.Optional && !result {
			return t, fmt.Errorf("optional String input requires a nullable marshalling adapter")
		}
		if result {
			t.ABI = "UnsafeMutableRawPointer?"
			t.Role = "owned"
		} else {
			t.ABI = "UnsafePointer<CChar>?"
			t.Role = "cstring"
		}
		return t, nil
	}
	var typ frameworkSymbol
	for _, f := range fragments {
		if f.Kind == "typeIdentifier" {
			if s, ok := symbols[f.PreciseIdentifier]; ok {
				typ = s
				break
			}
		}
	}
	if typ.Kind.Identifier == "swift.enum" && typ.EnumABI != "" && !t.Optional && (base == strings.Join(typ.Path, ".") || base == typ.Path[len(typ.Path)-1]) {
		t.Swift = strings.Join(typ.Path, ".")
		t.EnumABI = typ.EnumABI
		t.ABI = typ.EnumABI
		return t, nil
	}
	if typ.Kind.Identifier == "swift.class" && (base == strings.Join(typ.Path, ".") || base == typ.Path[len(typ.Path)-1]) {
		t.Swift = strings.Join(typ.Path, ".")
		t.ABI = "UnsafeMutableRawPointer?"
		t.Role = "object"
		if result {
			t.Role = "owned"
		}
		return t, nil
	}
	return t, fmt.Errorf("unsupported framework type %q", spelling)
}

func (t frameworkType) input(name string) string {
	if t.Role == "cstring" {
		return "text(" + name + ")"
	}
	if t.Role == "object" {
		if t.Optional {
			return name + ".map { borrow($0, as: " + t.Swift + ".self) }"
		}
		return "borrow(" + name + ", as: " + t.Swift + ".self)"
	}
	if t.EnumABI != "" {
		return t.Swift + "(rawValue: .init(" + name + "))!"
	}
	if t.Swift != t.ABI {
		return t.Swift + "(" + name + ")"
	}
	return name
}

func (t frameworkType) output(expr string) string {
	if t.Role == "owned" {
		if strings.TrimSuffix(t.Swift, "?") == "String" {
			if t.Optional {
				return "own((" + expr + ").map { $0 as NSString })"
			}
			return "own(" + expr + " as NSString)"
		}
		return "own(" + expr + ")"
	}
	if t.EnumABI != "" {
		return t.ABI + "((" + expr + ").rawValue)"
	}
	if t.Swift != t.ABI {
		return t.ABI + "(" + expr + ")"
	}
	return expr
}

func frameworkAllowed(s frameworkSymbol) error {
	if s.ExternalSuperclass != "" {
		return fmt.Errorf("external superclass %s requires an executor adapter", s.ExternalSuperclass)
	}
	for _, a := range s.Availability {
		if a.Unavailable || ((a.Domain == "Swift" || a.Domain == "macOS") && a.Obsoleted != nil) {
			return fmt.Errorf("unavailable declaration")
		}
	}
	for _, f := range s.Fragments {
		if f.Spelling == "async" || strings.Contains(f.Spelling, "Actor") || f.Spelling == "isolated" {
			return fmt.Errorf("async or actor-isolated declaration requires an adapter")
		}
	}
	return nil
}

func frameworkBindingName(s frameworkSymbol, setter bool) string {
	prefix := "Framework_"
	if setter {
		prefix += "Set_"
	}
	path := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return '_'
	}, strings.Join(s.Path, "_"))
	hash := sha256.Sum256([]byte(s.Identifier.Precise))
	return fmt.Sprintf("%s%s_%x", prefix, strings.TrimRight(path, "_"), hash[:4])
}

func generateAllFramework(symbols []frameworkSymbol) (string, []operation, []string, error) {
	symbols = append([]frameworkSymbol(nil), symbols...)
	sort.Slice(symbols, func(i, j int) bool { return symbols[i].Identifier.Precise < symbols[j].Identifier.Precise })
	byPath := map[string]frameworkSymbol{}
	byID := map[string]frameworkSymbol{}
	for _, symbol := range symbols {
		byID[symbol.Identifier.Precise] = symbol
		switch symbol.Kind.Identifier {
		case "swift.class", "swift.struct", "swift.enum", "swift.protocol":
			path := strings.Join(symbol.Path, ".")
			if previous, exists := byPath[path]; exists && previous.Identifier.Precise != symbol.Identifier.Precise {
				return "", nil, nil, fmt.Errorf("ambiguous framework type %s", path)
			}
			byPath[path] = symbol
		}
	}
	var source strings.Builder
	source.WriteString("import Foundation\nimport Virtualization\n\n")
	var ops []operation
	var skipped []string
	used := map[string]string{}
	for _, s := range symbols {
		switch s.Kind.Identifier {
		case "swift.property", "swift.type.property", "swift.init", "swift.method", "swift.type.method", "swift.subscript", "swift.type.subscript", "swift.func":
		default:
			continue
		}
		setters := []bool{false}
		if (s.Kind.Identifier == "swift.property" || s.Kind.Identifier == "swift.type.property") && strings.Contains(frameworkSpelling(s.Fragments), " set") {
			setters = append(setters, true)
		}
		for _, setter := range setters {
			name := frameworkBindingName(s, setter)
			if previous, ok := used[name]; ok {
				return "", nil, nil, fmt.Errorf("duplicate framework binding %s for %s and %s", name, previous, s.Identifier.Precise)
			}
			used[name] = s.Identifier.Precise
			generated, call, err := generateFrameworkMember(s, setter, byPath, byID)
			if err != nil {
				action := ""
				if setter {
					action = " setter"
				}
				path := strings.Join(s.Path, ".")
				skipped = append(skipped, path+action+": "+strings.TrimPrefix(err.Error(), path+": "))
				continue
			}
			source.WriteString(generated)
			ops = append(ops, call)
		}
	}
	sort.Strings(skipped)
	return source.String(), ops, skipped, nil
}

func generateFrameworkMember(s frameworkSymbol, setter bool, byPath, byID map[string]frameworkSymbol) (string, operation, error) {
	path := strings.Join(s.Path, ".")
	var out strings.Builder
	if len(s.Path) < 2 {
		return "", operation{}, fmt.Errorf("%s is not a member", path)
	}
	if err := frameworkAllowed(s); err != nil {
		return "", operation{}, fmt.Errorf("%s: %w", path, err)
	}
	name := frameworkBindingName(s, setter)
	symbol := "vz_" + name

	owner := strings.Join(s.Path[:len(s.Path)-1], ".")
	member := s.Path[len(s.Path)-1]
	version := frameworkVersion{Major: 11}
	for i := 1; i <= len(s.Path); i++ {
		enclosing, found := byPath[strings.Join(s.Path[:i], ".")]
		if i == len(s.Path) {
			enclosing, found = s, true
		}
		if !found {
			return "", operation{}, fmt.Errorf("framework type %q not found", strings.Join(s.Path[:i], "."))
		}
		if err := frameworkAllowed(enclosing); err != nil {
			return "", operation{}, fmt.Errorf("%s: %w", path, err)
		}
		for _, a := range enclosing.Availability {
			if a.Domain == "macOS" && a.Introduced != nil {
				v := *a.Introduced
				if v.Major > version.Major || v.Major == version.Major && (v.Minor > version.Minor || v.Minor == version.Minor && v.Patch > version.Patch) {
					version = v
				}
			}
		}
	}
	op := operation{Symbol: symbol, Name: name, Parameters: map[string]string{}}
	var parameters []string
	receiver := owner
	kind := s.Kind.Identifier
	if kind == "swift.property" || kind == "swift.method" {
		if byPath[owner].Kind.Identifier != "swift.class" {
			return "", operation{}, fmt.Errorf("%s: instance receiver is not a class", path)
		}
		parameters = append(parameters, "_ receiver: UnsafeMutableRawPointer?")
		op.Parameters["receiver"] = "object"
		receiver = "borrow(receiver, as: " + owner + ".self)"
	}
	var result frameworkType
	var expr string
	switch kind {
	case "swift.property", "swift.type.property":
		propertyType, err := frameworkValueType(frameworkTypeFragments(s.Fragments), byID, !setter)
		if err != nil {
			return "", operation{}, fmt.Errorf("%s: %w", path, err)
		}
		expr = receiver + "." + member
		if setter {
			parameters = append(parameters, "_ value: "+propertyType.ABI)
			if propertyType.Role != "" {
				op.Parameters["value"] = propertyType.Role
			}
			expr += " = " + propertyType.input("value")
		} else {
			result = propertyType
		}
	case "swift.init", "swift.method", "swift.type.method":
		var args []string
		labels := strings.Split(strings.TrimSuffix(strings.SplitN(member, "(", 2)[1], ")"), ":")
		for i, p := range s.Signature.Parameters {
			typ, err := frameworkValueType(frameworkTypeFragments(p.Fragments), byID, false)
			if err != nil {
				return "", operation{}, fmt.Errorf("%s parameter %s: %w", path, p.Name, err)
			}
			name := fmt.Sprintf("arg%d", i)
			parameters = append(parameters, "_ "+name+": "+typ.ABI)
			if typ.Role != "" {
				op.Parameters[name] = typ.Role
			}
			if i >= len(labels)-1 {
				return "", operation{}, fmt.Errorf("%s has inconsistent parameter labels", path)
			}
			label := labels[i]
			arg := typ.input(name)
			if label != "_" {
				arg = label + ": " + arg
			}
			args = append(args, arg)
		}
		if kind == "swift.init" {
			if byPath[owner].Kind.Identifier != "swift.class" {
				return "", operation{}, fmt.Errorf("%s: initializer is not a class", path)
			}
			result = frameworkType{Swift: owner, ABI: "UnsafeMutableRawPointer?", Role: "owned"}
			expr = owner + "(" + strings.Join(args, ", ") + ")"
		} else {
			var err error
			returns := append([]frameworkFragment(nil), s.Signature.Returns...)
			for i, fragment := range returns {
				if fragment.Kind == "typeIdentifier" && fragment.Spelling == "Self" {
					returns[i].Spelling = owner
					returns[i].PreciseIdentifier = byPath[owner].Identifier.Precise
				}
			}
			result, err = frameworkValueType(returns, byID, true)
			if err != nil {
				return "", operation{}, fmt.Errorf("%s: %w", path, err)
			}
			expr = receiver + "." + strings.SplitN(member, "(", 2)[0] + "(" + strings.Join(args, ", ") + ")"
		}
	default:
		return "", operation{}, fmt.Errorf("%s: unsupported declaration kind %s", path, kind)
	}

	throws := false
	for _, f := range s.Fragments {
		throws = throws || f.Kind == "keyword" && f.Spelling == "throws"
	}
	if throws {
		parameters = append(parameters, "_ errorOut: UnsafeMutablePointer<UnsafeMutableRawPointer?>?")
		op.Parameters["errorOut"] = "error-out"
		expr = "try " + expr
	}
	if len(parameters) > 15 {
		return "", operation{}, fmt.Errorf("%s: bridge ABI supports at most 15 arguments including receiver and error output", path)
	}
	returnDecl := ""
	if result.ABI != "" {
		returnDecl = " -> " + result.ABI
	}
	fmt.Fprintf(&out, "@c(%s)\npublic func %s(%s)%s {\n    bridgeSync {\n        guard #available(macOS %d.%d.%d, *) else { preconditionFailure(\"API requires macOS %d.%d.%d\") }\n", symbol, symbol, strings.Join(parameters, ", "), returnDecl, version.Major, version.Minor, version.Patch, version.Major, version.Minor, version.Patch)
	body := expr
	if result.ABI != "" {
		body = "return " + result.output(expr)
	}
	if throws {
		fallback := ""
		if result.Role == "owned" {
			fallback = " return nil"
		} else if result.ABI == "Bool" {
			fallback = " return false"
		} else if result.ABI != "" {
			fallback = " return 0"
		}
		fmt.Fprintf(&out, "        errorOut?.pointee = nil\n        do { %s } catch { fail(error, errorOut);%s }\n", body, fallback)
	} else {
		fmt.Fprintf(&out, "        %s\n", body)
	}
	out.WriteString("    }\n}\n\n")
	op.Result = result.Role
	return out.String(), op, nil
}
