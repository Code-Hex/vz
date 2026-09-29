package main

import (
	"encoding/json"
	"fmt"
	"go/token"
	"io"
	"regexp"
	"strings"
)

type operation struct {
	Symbol        string
	Name          string
	Parameters    map[string]string
	Result        string
	Architectures []string
}

type clangType struct {
	QualType string `json:"qualType"`
}
type node struct {
	Kind  string    `json:"kind"`
	Name  string    `json:"name"`
	Type  clangType `json:"type"`
	Inner []node    `json:"inner"`
}
type valueType struct{ C, Go, ABI, Role string }
type argument struct {
	Name string
	valueType
}
type checkedOperation struct {
	Symbol, Name string
	Parameters   []argument
	Result       valueType
}

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func checkHeader(input io.Reader, contracts []operation, arch string) ([]checkedOperation, error) {
	declarations := map[string]node{}
	var visit func(node) error
	visit = func(n node) error {
		if n.Kind == "FunctionDecl" && strings.HasPrefix(n.Name, "vz_") {
			if old, ok := declarations[n.Name]; ok && old.Type.QualType != n.Type.QualType {
				return fmt.Errorf("conflicting declaration %s", n.Name)
			}
			declarations[n.Name] = n
		}
		for _, child := range n.Inner {
			if err := visit(child); err != nil {
				return err
			}
		}
		return nil
	}
	decoder := json.NewDecoder(input)
	for {
		var n node
		err := decoder.Decode(&n)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if err := visit(n); err != nil {
			return nil, err
		}
	}
	var checked []checkedOperation
	names := map[string]bool{"SDKVersion": true}
	symbols := map[string]bool{}
	for _, contract := range contracts {
		enabled := len(contract.Architectures) == 0
		for _, a := range contract.Architectures {
			if a != "arm64" && a != "amd64" {
				return nil, fmt.Errorf("invalid architecture %q", a)
			}
			enabled = enabled || a == arch
		}
		if !enabled {
			continue
		}
		if !identifier.MatchString(contract.Name) || contract.Name[0] < 'A' || contract.Name[0] > 'Z' || token.Lookup(contract.Name).IsKeyword() || names[contract.Name] {
			return nil, fmt.Errorf("invalid or duplicate operation name %q", contract.Name)
		}
		if symbols[contract.Symbol] {
			return nil, fmt.Errorf("duplicate symbol %s", contract.Symbol)
		}
		names[contract.Name] = true
		symbols[contract.Symbol] = true
		declaration, ok := declarations[contract.Symbol]
		if !ok {
			return nil, fmt.Errorf("missing compiler declaration %s", contract.Symbol)
		}
		if strings.Contains(declaration.Type.QualType, "...") {
			return nil, fmt.Errorf("variadic C export %s is unsupported", contract.Symbol)
		}
		position := strings.IndexByte(declaration.Type.QualType, '(')
		if position < 0 {
			return nil, fmt.Errorf("invalid function type %s", declaration.Type.QualType)
		}
		result, err := translate(strings.TrimSpace(declaration.Type.QualType[:position]), contract.Result, true)
		if err != nil {
			return nil, fmt.Errorf("%s result: %w", contract.Name, err)
		}
		call := checkedOperation{Symbol: contract.Symbol, Name: contract.Name, Result: result}
		used := map[string]bool{}
		for _, parameter := range declaration.Inner {
			if parameter.Kind != "ParmVarDecl" {
				continue
			}
			role := contract.Parameters[parameter.Name]
			typ, err := translate(parameter.Type.QualType, role, false)
			if err != nil {
				return nil, fmt.Errorf("%s parameter %s: %w", contract.Name, parameter.Name, err)
			}
			if typ.Go == "" {
				return nil, fmt.Errorf("void argument")
			}
			used[parameter.Name] = true
			call.Parameters = append(call.Parameters, argument{Name: parameter.Name, valueType: typ})
		}
		for name := range contract.Parameters {
			if !used[name] {
				return nil, fmt.Errorf("%s has no parameter %s", contract.Name, name)
			}
		}
		if len(call.Parameters) > 15 {
			return nil, fmt.Errorf("%s exceeds purego argument limit", contract.Name)
		}
		checked = append(checked, call)
	}
	for symbol := range declarations {
		if symbol == "vz_bridge_abi" {
			continue
		}
		if !symbols[symbol] {
			return nil, fmt.Errorf("compiler export %s has no active contract", symbol)
		}
	}
	return checked, nil
}

func translate(c, role string, result bool) (valueType, error) {
	for _, word := range []string{"_Nullable", "_Nonnull", "_Null_unspecified"} {
		c = strings.ReplaceAll(c, word, "")
	}
	c = strings.NewReplacer("NSInteger", "intptr_t", "NSUInteger", "uintptr_t").Replace(c)
	ctype := strings.Join(strings.Fields(c), " ")
	for _, word := range []string{"const", "volatile"} {
		c = strings.ReplaceAll(c, word, "")
	}
	c = strings.Join(strings.Fields(c), " ")
	c = strings.ReplaceAll(c, " *", "*")
	c = strings.ReplaceAll(c, "* ", "*")
	scalar := map[string]string{"void": "", "_Bool": "bool", "bool": "bool", "int8_t": "int8", "uint8_t": "uint8", "int16_t": "int16", "uint16_t": "uint16", "int32_t": "int32", "uint32_t": "uint32", "int64_t": "int64", "uint64_t": "uint64", "int": "int32", "unsigned int": "uint32", "long": "int64", "unsigned long": "uint64", "long long": "int64", "unsigned long long": "uint64", "float": "float32", "double": "float64", "intptr_t": "int64", "uintptr_t": "uint64"}
	if typ, ok := scalar[c]; ok {
		if role != "" {
			return valueType{}, fmt.Errorf("scalar %s cannot have role %s", c, role)
		}
		return valueType{C: ctype, Go: typ, ABI: typ}, nil
	}
	if !strings.Contains(c, "*") {
		return valueType{}, fmt.Errorf("unsupported compiler C type %s", c)
	}
	switch role {
	case "object":
		if result || c != "void*" {
			break
		}
		return valueType{C: ctype, Go: "objc.NSObject", ABI: "unsafe.Pointer", Role: role}, nil
	case "owned":
		if !result || c != "void*" {
			break
		}
		return valueType{C: ctype, Go: "*objc.Pointer", ABI: "unsafe.Pointer", Role: role}, nil
	case "cstring":
		if result || c != "char*" {
			break
		}
		return valueType{C: ctype, Go: "string", ABI: "string", Role: role}, nil
	case "error-out":
		if result || c != "void**" {
			break
		}
		return valueType{C: ctype, Go: "*unsafe.Pointer", ABI: "*unsafe.Pointer", Role: role}, nil
	case "raw":
		return valueType{C: ctype, Go: "unsafe.Pointer", ABI: "unsafe.Pointer", Role: role}, nil
	}
	return valueType{}, fmt.Errorf("invalid or missing role %q for %s", role, c)
}
