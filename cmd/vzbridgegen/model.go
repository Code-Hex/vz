package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type sdkReport struct {
	Schema     int
	SDKVersion string
	Compiler   string
	Targets    []sdkTarget
}
type sdkTarget struct {
	Architecture string
	Triple       string
	SourceSHA256 string
	Classes      []sdkClass
	Methods      []sdkMethod
	Enums        []sdkEnum
	Declarations []sdkDeclaration
}
type sdkClass struct {
	Name, Super, Introduced string
	Unavailable             bool
}
type sdkType struct {
	Spelling     string
	Canonical    string
	Kind         string
	Class        string
	Protocols    []string
	Generics     []string
	Nullability  []string
	Qualifiers   []string
	PointerDepth int
	ABI          string
	Go           string
}
type sdkParameter struct {
	Name     string
	Type     sdkType
	Consumed bool
}
type sdkMethod struct {
	ID            string
	Owner         string
	Selector      string
	GoName        string
	Instance      bool
	Factory       bool
	Implicit      bool
	InheritedFrom string
	Category      string
	Family        string
	Result        sdkType
	Parameters    []sdkParameter
	Ownership     string
	ConsumesSelf  bool
	Introduced    string
	Deprecated    string
	Obsoleted     string
	Unavailable   bool
	Unsupported   []string
}
type sdkEnum struct {
	Name, ABI string
	Values    []sdkEnumValue
}
type sdkEnumValue struct{ Name, Value string }
type sdkDeclaration struct{ Kind, Name, Type, Disposition, Reason string }

type clangType struct{ QualType, DesugaredQualType string }
type clangPosition struct{ Offset, TokLen int }
type clangNode struct {
	Kind, Name                               string
	Value                                    json.RawMessage
	Type, ReturnType, FixedUnderlyingType    clangType
	Inner                                    []clangNode
	Instance, Variadic, IsImplicit, Implicit bool
	Super                                    struct{ Name string }
	Interface                                struct{ Name string }
	Protocols                                []struct{ Name string }
	Range                                    struct{ Begin, End clangPosition }
}
type sdkAttributes struct {
	Introduced, Deprecated, Obsoleted, Family                  string
	Unavailable, Retained, NotRetained, ConsumesSelf, Consumed bool
	Unknown                                                    []string
}

var qualifierPattern = regexp.MustCompile(`\b(const|volatile|restrict|_Nullable|_Nonnull|_Null_unspecified|__strong|__weak|__autoreleasing|__unsafe_unretained)\b`)
var genericPattern = regexp.MustCompile(`<([^<>]*(?:<[^<>]*>[^<>]*)*)>`)
var scalarABI = map[string]string{"void": "", "bool": "bool", "_Bool": "bool", "char": "int8", "signed char": "int8", "unsigned char": "uint8", "short": "int16", "unsigned short": "uint16", "int": "int32", "unsigned int": "uint32", "long": "int64", "unsigned long": "uint64", "long long": "int64", "unsigned long long": "uint64", "float": "float32", "double": "float64"}

func canonicalType(t clangType) string {
	value := t.DesugaredQualType
	if value == "" {
		value = t.QualType
	}
	return strings.Join(strings.Fields(qualifierPattern.ReplaceAllString(value, "")), " ")
}
func resolveAlias(value string, aliases map[string]string) string {
	seen := map[string]bool{}
	for !seen[value] && value != "id" && value != "Class" {
		next, ok := aliases[value]
		if !ok {
			return value
		}
		seen[value] = true
		value = next
	}
	return value
}
func classifyType(t clangType, aliases map[string]string, classes map[string]bool, enums map[string]string, result bool) (sdkType, string) {
	typ := sdkType{Spelling: t.QualType, Canonical: resolveAlias(canonicalType(t), aliases)}
	for _, qualifier := range qualifierPattern.FindAllString(t.QualType, -1) {
		if strings.HasPrefix(qualifier, "_N") {
			typ.Nullability = append(typ.Nullability, qualifier)
		} else {
			typ.Qualifiers = append(typ.Qualifiers, qualifier)
		}
	}
	typ.PointerDepth = strings.Count(genericPattern.ReplaceAllString(typ.Canonical, ""), "*")
	unsupported := func(reason string) (sdkType, string) {
		typ.Kind = "unsupported"
		return typ, reason + ": " + typ.Canonical
	}
	if strings.Contains(t.QualType, "va_list") {
		return unsupported("va_list requires native variadic adapter")
	}
	if strings.Contains(typ.Canonical, "^") {
		return unsupported("Objective-C block needs a native adapter")
	}
	if strings.Contains(typ.Canonical, "(") {
		return unsupported("function pointer is unsupported")
	}
	if t.QualType == "BOOL" {
		typ.Kind = "scalar"
		typ.ABI = "bool"
		typ.Go = "bool"
		return typ, ""
	}
	if abi, ok := scalarABI[typ.Canonical]; ok {
		typ.Kind = "scalar"
		typ.ABI = abi
		typ.Go = abi
		return typ, ""
	}
	if abi, ok := enums[strings.TrimPrefix(typ.Canonical, "enum ")]; ok {
		typ.Kind = "enum"
		typ.ABI = abi
		typ.Go = abi
		return typ, ""
	}
	for _, match := range genericPattern.FindAllStringSubmatch(typ.Canonical, -1) {
		typ.Generics = append(typ.Generics, match[1])
	}
	base := strings.TrimSpace(genericPattern.ReplaceAllString(typ.Canonical, ""))
	if base == "id" {
		typ.Kind = "object"
		typ.Class = "id"
		typ.Protocols = typ.Generics
		typ.Generics = nil
		typ.ABI = "unsafe.Pointer"
		typ.Go = "objc.NSObject"
		if result {
			typ.Go = "*objc.Pointer"
		}
		return typ, ""
	}
	if base == "Class" {
		typ.Kind = "class"
		typ.ABI = "unsafe.Pointer"
		typ.Go = "unsafe.Pointer"
		return typ, ""
	}
	if strings.HasSuffix(base, "*") {
		inner := strings.TrimSpace(strings.TrimSuffix(base, "*"))
		if classes[inner] {
			typ.Kind = "object"
			typ.Class = inner
			typ.ABI = "unsafe.Pointer"
			typ.Go = "objc.NSObject"
			if result {
				typ.Go = "*objc.Pointer"
			}
			return typ, ""
		}
		if strings.HasSuffix(inner, "*") && strings.TrimSpace(strings.TrimSuffix(inner, "*")) == "NSError" {
			typ.Kind = "error-out"
			typ.Class = "NSError"
			typ.ABI = "*unsafe.Pointer"
			typ.Go = "*unsafe.Pointer"
			return typ, ""
		}
		inner = resolveAlias(inner, aliases)
		if inner == "void" {
			typ.Kind = "pointer"
			typ.ABI = "unsafe.Pointer"
			typ.Go = "unsafe.Pointer"
			return typ, ""
		}
		if inner == "char" && strings.Contains(t.QualType, "const") {
			typ.Kind = "cstring"
			typ.ABI = "string"
			typ.Go = "string"
			return typ, ""
		}
		if abi, ok := scalarABI[inner]; ok && abi != "" {
			typ.Kind = "pointer"
			typ.ABI = "*" + abi
			typ.Go = "*" + abi
			return typ, ""
		}
		return unsupported("opaque or object pointer needs an explicit policy")
	}
	return unsupported("aggregate or unresolved value type")
}

func methodFamily(selector, override string) string {
	if override != "" {
		return override
	}
	name := strings.TrimLeft(strings.Split(selector, ":")[0], "_")
	for _, family := range []string{"mutableCopy", "alloc", "copy", "init", "new"} {
		if strings.HasPrefix(name, family) && (len(name) == len(family) || name[len(family)] < 'a' || name[len(family)] > 'z') {
			return family
		}
	}
	return "none"
}
func methodName(owner, selector string) string {
	parts := strings.Split(strings.TrimSuffix(selector, ":"), ":")
	for i, part := range parts {
		if part != "" {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return owner + "_" + strings.Join(parts, "_")
}
func stableMethods(methods []sdkMethod) ([]sdkMethod, error) {
	unique := map[string]sdkMethod{}
	for _, method := range methods {
		previous, exists := unique[method.ID]
		if exists {
			if previous.Result.Canonical != method.Result.Canonical || len(previous.Parameters) != len(method.Parameters) {
				return nil, fmt.Errorf("conflicting SDK declaration %s", method.ID)
			}
			for i, p := range method.Parameters {
				if p.Type.Canonical != previous.Parameters[i].Type.Canonical {
					return nil, fmt.Errorf("conflicting parameter in %s", method.ID)
				}
			}
			if previous.Unavailable != method.Unavailable || previous.Ownership != method.Ownership || previous.Family != method.Family {
				return nil, fmt.Errorf("conflicting ownership or availability in %s: %#v versus %#v", method.ID, previous, method)
			}
			previous.Introduced = later(previous.Introduced, method.Introduced)
			previous.Deprecated = earlier(previous.Deprecated, method.Deprecated)
			previous.Obsoleted = earlier(previous.Obsoleted, method.Obsoleted)
			previous.Unsupported = append(previous.Unsupported, method.Unsupported...)
			unique[method.ID] = previous
		} else {
			unique[method.ID] = method
		}
	}
	methods = nil
	names := map[string]int{}
	for _, m := range unique {
		names[m.GoName]++
	}
	for _, m := range unique {
		if names[m.GoName] > 1 {
			if !m.Instance {
				m.GoName += "_Class"
			}
		}
		methods = append(methods, m)
	}
	sort.Slice(methods, func(i, j int) bool { return methods[i].ID < methods[j].ID })
	return methods, nil
}
