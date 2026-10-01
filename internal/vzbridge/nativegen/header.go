package main

import (
	"encoding/json"
	"fmt"
	"go/token"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

type clangType struct {
	QualType string `json:"qualType"`
}

type sourceLocation struct {
	Offset           int             `json:"offset"`
	TokenLength      int             `json:"tokLen"`
	SpellingLocation *sourceLocation `json:"spellingLoc"`
	IncludedFrom     *sourceLocation `json:"includedFrom"`
}

type declaration struct {
	Kind     string        `json:"kind"`
	Name     string        `json:"name"`
	Type     clangType     `json:"type"`
	Inner    []declaration `json:"inner"`
	Variadic bool          `json:"variadic"`
	Range    struct {
		Begin sourceLocation `json:"begin"`
		End   sourceLocation `json:"end"`
	} `json:"range"`
}

type alias struct{ underlying, role string }

func discoverBindings(path string) ([]binding, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	command := exec.Command("xcrun", "clang", "-x", "c-header", "-fsyntax-only", "-Werror", "-Xclang", "-ast-dump=json", path)
	var diagnostics strings.Builder
	command.Stderr = &diagnostics
	ast, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("clang: %w: %s", err, diagnostics.String())
	}
	var root declaration
	if err := json.Unmarshal(ast, &root); err != nil {
		return nil, fmt.Errorf("clang AST: %w", err)
	}
	aliases := make(map[string]alias)
	for _, node := range root.Inner {
		if node.Kind != "TypedefDecl" {
			continue
		}
		role, err := annotation(node, source)
		if err != nil {
			return nil, err
		}
		if role != "" && role != "object" && role != "owned" && role != "errorout" {
			return nil, fmt.Errorf("%s: unknown annotation %q for typedef", node.Name, role)
		}
		aliases[node.Name] = alias{underlying: node.Type.QualType, role: role}
	}
	for name, a := range aliases {
		if a.role == "" {
			continue
		}
		canonical, _, err := resolveType(a.underlying, aliases)
		if err != nil {
			return nil, err
		}
		expected := "void*"
		if a.role == "errorout" {
			expected = "void**"
		}
		if canonical != expected {
			return nil, fmt.Errorf("%s: %s requires %s ABI, got %s", name, a.role, strings.ReplaceAll(expected, "*", " *"), canonical)
		}
	}
	var bindings []binding
	names, symbols := make(map[string]bool), make(map[string]bool)
	for _, node := range root.Inner {
		if node.Kind != "FunctionDecl" || !strings.HasPrefix(node.Name, "vz_") {
			continue
		}
		name := strings.TrimPrefix(node.Name, "vz_")
		r, size := utf8.DecodeRuneInString(name)
		name = string(unicode.ToUpper(r)) + name[size:]
		if !token.IsIdentifier(name) || !token.IsExported(name) {
			return nil, fmt.Errorf("invalid binding name for %s", node.Name)
		}
		if symbols[node.Name] {
			return nil, fmt.Errorf("duplicate native symbol %s", node.Name)
		}
		if names[name] {
			return nil, fmt.Errorf("duplicate Go name %s", name)
		}
		symbols[node.Name], names[name] = true, true
		metadata, err := annotation(node, source)
		if err != nil {
			return nil, err
		}
		if metadata != "" && metadata != "noqueue" && metadata != "manual" {
			return nil, fmt.Errorf("%s: unknown annotation %q for function", node.Name, metadata)
		}
		if node.Variadic {
			return nil, fmt.Errorf("%s: variadic native ABI is unsupported", node.Name)
		}
		if strings.Contains(node.Type.QualType, "__attribute__") {
			return nil, fmt.Errorf("%s: unsupported function type %q", node.Name, node.Type.QualType)
		}
		result, _, ok := strings.Cut(node.Type.QualType, "(")
		if !ok {
			return nil, fmt.Errorf("%s: unsupported function type %q", node.Name, node.Type.QualType)
		}
		goResult, err := goType(result, true, aliases)
		if err != nil {
			return nil, fmt.Errorf("%s result: %w", node.Name, err)
		}
		b := binding{Name: name, Symbol: node.Name, Result: goResult, Queue: metadata == ""}
		parameters := make(map[string]bool)
		for _, child := range node.Inner {
			if child.Kind != "ParmVarDecl" {
				continue
			}
			if !token.IsIdentifier(child.Name) || child.Name == "_" || parameters[child.Name] {
				return nil, fmt.Errorf("%s: invalid parameter %q", node.Name, child.Name)
			}
			parameters[child.Name] = true
			if attr, err := annotation(child, source); err != nil {
				return nil, err
			} else if attr != "" {
				return nil, fmt.Errorf("%s: unsupported parameter annotation %q", node.Name, attr)
			}
			typ, err := goType(child.Type.QualType, false, aliases)
			if err != nil {
				return nil, fmt.Errorf("%s parameter %s: %w", node.Name, child.Name, err)
			}
			b.Parameters = append(b.Parameters, parameter{Name: child.Name, Type: typ})
		}
		if metadata != "manual" {
			bindings = append(bindings, b)
		}
	}
	if len(bindings) == 0 {
		return nil, fmt.Errorf("no native bindings found in %s", path)
	}
	return bindings, nil
}

// Clang JSON omits annotation text, so read the exact source range it provides.
var annotationPattern = regexp.MustCompile(`^annotate\s*\(\s*"vzbridge:([a-z]+)"\s*\)$`)

func annotation(node declaration, source []byte) (string, error) {
	var value string
	for _, attr := range node.Inner {
		if attr.Kind != "AnnotateAttr" {
			continue
		}
		if value != "" {
			return "", fmt.Errorf("%s: multiple annotations", node.Name)
		}
		begin, end := attr.Range.Begin, attr.Range.End
		if begin.SpellingLocation != nil || end.SpellingLocation != nil || begin.IncludedFrom != nil || end.IncludedFrom != nil {
			return "", fmt.Errorf("%s: annotations must be written directly in the header", node.Name)
		}
		limit := end.Offset + end.TokenLength
		if begin.Offset < 0 || limit <= begin.Offset || limit > len(source) {
			return "", fmt.Errorf("%s: invalid annotation source range", node.Name)
		}
		text := string(source[begin.Offset:limit])
		match := annotationPattern.FindStringSubmatch(text)
		if match == nil {
			return "", fmt.Errorf("%s: unknown annotation %q", node.Name, text)
		}
		value = match[1]
	}
	return value, nil
}

func normalizeType(typ string) string {
	return strings.ReplaceAll(strings.Join(strings.Fields(typ), " "), " *", "*")
}

func resolveType(typ string, aliases map[string]alias) (string, string, error) {
	typ = normalizeType(typ)
	seen := make(map[string]bool)
	role := ""
	for {
		a, ok := aliases[typ]
		if !ok {
			return typ, role, nil
		}
		if seen[typ] {
			return "", "", fmt.Errorf("cyclic typedef %s", typ)
		}
		seen[typ] = true
		if a.role != "" {
			if role != "" && role != a.role {
				return "", "", fmt.Errorf("conflicting ownership in typedef %s", typ)
			}
			role = a.role
		}
		typ = normalizeType(a.underlying)
	}
}

// These are the C scalar ABI types on Darwin's supported 64-bit targets.
var scalarTypes = map[string]string{
	"void": "", "_Bool": "bool", "bool": "bool",
	"char": "int8", "signed char": "int8", "unsigned char": "uint8",
	"short": "int16", "unsigned short": "uint16",
	"int": "int32", "unsigned int": "uint32",
	"long": "int64", "unsigned long": "uint64",
	"long long": "int64", "unsigned long long": "uint64",
	"float": "float32", "double": "float64",
}

func goType(typ string, result bool, aliases map[string]alias) (string, error) {
	canonical, role, err := resolveType(typ, aliases)
	if err != nil {
		return "", err
	}
	switch role {
	case "errorout":
		if result {
			return "", fmt.Errorf("errorout type is only valid as a parameter")
		}
		return "*unsafe.Pointer", nil
	case "object":
		if result {
			return "", fmt.Errorf("object type is only valid as a parameter")
		}
		return "objc.NSObject", nil
	case "owned":
		if !result {
			return "", fmt.Errorf("owned type is only valid as a result")
		}
		return "*objc.Pointer", nil
	}
	if scalar, ok := scalarTypes[canonical]; ok && (result || scalar != "") {
		return scalar, nil
	}
	switch canonical {
	case "void*", "const void*":
		return "unsafe.Pointer", nil
	case "void**":
		return "*unsafe.Pointer", nil
	case "const char*":
		if !result {
			return "string", nil
		}
	}
	return "", fmt.Errorf("unsupported native ABI type %q (resolved %q)", typ, canonical)
}
