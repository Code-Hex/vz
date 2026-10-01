package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"golang.org/x/mod/semver"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var foundation = map[string]bool{"NSObject": true, "NSString": true, "NSURL": true, "NSData": true, "NSArray": true, "NSMutableArray": true, "NSDictionary": true, "NSMutableDictionary": true, "NSError": true, "NSUUID": true}

func selected(name string) bool { return strings.HasPrefix(name, "VZ") || foundation[name] }
func later(a, b string) string {
	if a == "" || semver.Compare("v"+a, "v"+b) < 0 {
		return b
	}
	return a
}
func attrText(n clangNode, source []byte) (string, error) {
	start, end := n.Range.Begin.Offset, n.Range.End.Offset+n.Range.End.TokLen
	if start < 0 || end > len(source) || start >= end {
		return "", fmt.Errorf("invalid %s source range", n.Kind)
	}
	return string(source[start:end]), nil
}

var availabilityAttr = regexp.MustCompile(`^availability\s*\(\s*([A-Za-z0-9_]+)\s*,(.*)\)$`)
var quotedAttr = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
var unavailableAttr = regexp.MustCompile(`(?:^|,)\s*unavailable\s*(?:,|$)`)
var versionAttr = regexp.MustCompile(`(introduced|deprecated|obsoleted)\s*=\s*([0-9.]+)`)
var familyAttr = regexp.MustCompile(`objc_method_family\s*\(\s*(\w+)\s*\)`)

func attributes(n clangNode, source []byte) (sdkAttributes, error) {
	a := sdkAttributes{}
	for _, child := range n.Inner {
		switch child.Kind {
		case "AvailabilityAttr":
			text, err := attrText(child, source)
			if err != nil {
				return a, err
			}
			match := availabilityAttr.FindStringSubmatch(text)
			if match == nil {
				return a, fmt.Errorf("unknown availability %q", text)
			}
			if match[1] != "macos" && match[1] != "macosx" {
				continue
			}
			arguments := quotedAttr.ReplaceAllString(match[2], `""`)
			for _, m := range versionAttr.FindAllStringSubmatch(arguments, -1) {
				switch m[1] {
				case "introduced":
					a.Introduced = m[2]
				case "deprecated":
					a.Deprecated = m[2]
				case "obsoleted":
					a.Obsoleted = m[2]
				}
			}
			a.Unavailable = a.Unavailable || unavailableAttr.MatchString(arguments)
		case "UnavailableAttr":
			a.Unavailable = true
		case "NSReturnsRetainedAttr":
			a.Retained = true
		case "NSReturnsNotRetainedAttr", "NSReturnsAutoreleasedAttr":
			a.NotRetained = true
		case "NSConsumesSelfAttr":
			a.ConsumesSelf = true
		case "NSConsumedAttr":
			a.Consumed = true
		case "ObjCMethodFamilyAttr":
			text, err := attrText(child, source)
			if err != nil {
				return a, err
			}
			m := familyAttr.FindStringSubmatch(text)
			if m == nil {
				return a, fmt.Errorf("unknown method family %q", text)
			}
			a.Family = m[1]
		default:
			if strings.Contains(child.Kind, "Consumed") || strings.Contains(child.Kind, "ReturnsRetained") || strings.Contains(child.Kind, "CallingConv") {
				a.Unknown = append(a.Unknown, child.Kind)
			}
		}
	}
	return a, nil
}
func clangRun(output string, args ...string) error {
	f, err := os.Create(output)
	if err != nil {
		return err
	}
	defer f.Close()
	cmd := exec.Command("xcrun", append([]string{"clang"}, args...)...)
	cmd.Stdout = f
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err = cmd.Run(); err != nil {
		return fmt.Errorf("clang: %w: %s", err, stderr.String())
	}
	return nil
}
func extractTarget(input, architecture, triple, directory string) (sdkTarget, error) {
	target := sdkTarget{Architecture: architecture, Triple: triple}
	sourcePath := filepath.Join(directory, architecture+".m")
	astPath := filepath.Join(directory, architecture+".json")
	args := []string{"-target", triple, "-x", "objective-c", "-fblocks", "-fobjc-arc"}
	if err := clangRun(sourcePath, append(args, "-E", input)...); err != nil {
		return target, err
	}
	if err := clangRun(astPath, append(args, "-fsyntax-only", "-Wno-everything", "-Xclang", "-ast-dump=json", sourcePath)...); err != nil {
		return target, err
	}
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		return target, err
	}
	// Preprocessor line markers contain local SDK paths, not semantic source.
	canonical := regexp.MustCompile(`(?m)^# [0-9]+ ".*".*$`).ReplaceAll(source, nil)
	hash := sha256.Sum256(canonical)
	target.SourceSHA256 = hex.EncodeToString(hash[:])
	f, err := os.Open(astPath)
	if err != nil {
		return target, err
	}
	defer f.Close()
	nodes, err := readDeclarations(f)
	if err != nil {
		return target, err
	}
	classes := map[string]bool{}
	aliases := map[string]string{}
	enums := map[string]string{}
	interfaces := map[string]clangNode{}
	protocols := map[string]clangNode{}
	for _, n := range nodes {
		switch n.Kind {
		case "ObjCInterfaceDecl":
			classes[n.Name] = true
			if len(n.Inner) > len(interfaces[n.Name].Inner) {
				interfaces[n.Name] = n
			}
		case "ObjCProtocolDecl":
			if len(n.Inner) > len(protocols[n.Name].Inner) {
				protocols[n.Name] = n
			}
		case "TypedefDecl":
			aliases[n.Name] = canonicalType(n.Type)
		}
	}
	for _, n := range nodes {
		if n.Kind == "EnumDecl" {
			base := resolveAlias(canonicalType(n.FixedUnderlyingType), aliases)
			if base == "" {
				continue
			}
			if abi, ok := scalarABI[base]; ok {
				enums[n.Name] = abi
			}
		}
	}
	for _, n := range nodes {
		if n.Kind == "EnumDecl" {
			var values []sdkEnumValue
			counter := new(big.Int)
			for _, v := range n.Inner {
				if v.Kind != "EnumConstantDecl" {
					continue
				}
				value := enumValue(v)
				if value != "" {
					if _, ok := counter.SetString(value, 10); !ok {
						return target, fmt.Errorf("invalid enum %s", v.Name)
					}
				}
				value = counter.String()
				counter.Add(counter, big.NewInt(1))
				if !strings.HasPrefix(n.Name, "VZ") && v.Name != "NSUTF8StringEncoding" {
					continue
				}
				values = append(values, sdkEnumValue{v.Name, value})
			}
			if len(values) > 0 {
				target.Enums = append(target.Enums, sdkEnum{Name: n.Name, ABI: enums[n.Name], Values: values})
			}
			continue
		}
		if n.Kind != "ObjCInterfaceDecl" && n.Kind != "ObjCCategoryDecl" && n.Kind != "ObjCProtocolDecl" {
			if strings.HasPrefix(n.Name, "VZ") || n.Kind == "VarDecl" && strings.HasPrefix(n.Name, "NS") {
				target.Declarations = append(target.Declarations, sdkDeclaration{n.Kind, n.Name, n.Type.QualType, "unsupported", "non-method declaration requires a native adapter"})
			}
			continue
		}
		owner := n.Name
		if n.Kind == "ObjCCategoryDecl" {
			owner = n.Interface.Name
		}
		if !selected(owner) || len(n.Inner) == 0 {
			continue
		}
		classAttr, err := attributes(n, source)
		if err != nil {
			return target, err
		}
		if def, ok := interfaces[owner]; ok && n.Kind == "ObjCCategoryDecl" {
			baseAttr, attrErr := attributes(def, source)
			err = attrErr
			classAttr = mergeAttributes(baseAttr, classAttr)
			if err != nil {
				return target, err
			}
		}
		if n.Kind == "ObjCInterfaceDecl" && len(n.Inner) == len(interfaces[owner].Inner) {
			target.Classes = append(target.Classes, sdkClass{owner, n.Super.Name, classAttr.Introduced, classAttr.Unavailable})
		}
		for _, child := range n.Inner {
			if child.Kind != "ObjCMethodDecl" {
				continue
			}
			m, err := extractMethod(owner, child, classAttr, source, aliases, classes, enums)
			if err != nil {
				return target, err
			}
			if n.Kind == "ObjCCategoryDecl" {
				m.Category = n.Name
			}
			if n.Kind == "ObjCProtocolDecl" {
				m.ID = "protocol:" + m.ID
				m.Unsupported = append(m.Unsupported, "protocol method requires a native delegate adapter")
			}
			target.Methods = append(target.Methods, m)
		}
	}
	// Adopted protocols carry their own availability into the concrete messages.
	for _, def := range nodes {
		owner := def.Name
		category := ""
		if def.Kind == "ObjCCategoryDecl" {
			owner = def.Interface.Name
			category = def.Name
		} else if def.Kind != "ObjCInterfaceDecl" {
			continue
		}
		if !selected(owner) || len(def.Protocols) == 0 {
			continue
		}
		ca, err := attributes(def, source)
		if err != nil {
			return target, err
		}
		if category != "" {
			base, err := attributes(interfaces[owner], source)
			if err != nil {
				return target, err
			}
			ca = mergeAttributes(base, ca)
		}
		seen := map[string]bool{}
		var adopt func(string, sdkAttributes) error
		adopt = func(name string, inherited sdkAttributes) error {
			if seen[name] {
				return nil
			}
			seen[name] = true
			p := protocols[name]
			pa, err := attributes(p, source)
			if err != nil {
				return err
			}
			pa = mergeAttributes(inherited, pa)
			for _, child := range p.Inner {
				if child.Kind != "ObjCMethodDecl" {
					continue
				}
				m, err := extractMethod(owner, child, pa, source, aliases, classes, enums)
				if err != nil {
					return err
				}
				exists := false
				for _, old := range target.Methods {
					if old.ID == m.ID {
						exists = true
						break
					}
				}
				if !exists {
					m.InheritedFrom = "protocol:" + name
					m.Category = category
					target.Methods = append(target.Methods, m)
				}
			}
			for _, parent := range p.Protocols {
				if err := adopt(parent.Name, pa); err != nil {
					return err
				}
			}
			return nil
		}
		for _, p := range def.Protocols {
			if err := adopt(p.Name, ca); err != nil {
				return target, err
			}
		}
	}
	// NSObject's inherited constructor belongs to each concrete class, unless init/new is unavailable.
	var lookup func(string, string, bool) (clangNode, string, bool)
	lookup = func(class, selector string, instance bool) (clangNode, string, bool) {
		n, ok := interfaces[class]
		if !ok {
			return clangNode{}, "", false
		}
		for _, m := range n.Inner {
			if m.Kind == "ObjCMethodDecl" && m.Name == selector && m.Instance == instance {
				return m, class, true
			}
		}
		if n.Super.Name != "" {
			return lookup(n.Super.Name, selector, instance)
		}
		return clangNode{}, "", false
	}
	for _, c := range target.Classes {
		n, owner, ok := lookup(c.Name, "new", false)
		if !ok || owner == c.Name {
			continue
		}
		a := sdkAttributes{Introduced: c.Introduced, Unavailable: c.Unavailable}
		m, err := extractMethod(c.Name, n, a, source, aliases, classes, enums)
		if err != nil {
			return target, err
		}
		m.InheritedFrom = owner
		if init, _, ok := lookup(c.Name, "init", true); ok {
			ia, err := attributes(init, source)
			if err != nil {
				return target, err
			}
			m.Introduced = later(m.Introduced, ia.Introduced)
			m.Deprecated = earlier(m.Deprecated, ia.Deprecated)
			m.Obsoleted = earlier(m.Obsoleted, ia.Obsoleted)
			if ia.Unavailable {
				m.Unsupported = append(m.Unsupported, "default init is unavailable")
			}
		}
		target.Methods = append(target.Methods, m)
	}
	target.Methods, err = stableMethods(target.Methods)
	if err != nil {
		return target, err
	}
	sort.Slice(target.Classes, func(i, j int) bool { return target.Classes[i].Name < target.Classes[j].Name })
	sort.Slice(target.Enums, func(i, j int) bool { return target.Enums[i].Name < target.Enums[j].Name })
	return target, nil
}
func readDeclarations(r io.Reader) ([]clangNode, error) {
	d := json.NewDecoder(r)
	if _, err := d.Token(); err != nil {
		return nil, err
	}
	var nodes []clangNode
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return nil, err
		}
		if key != "inner" {
			var discard json.RawMessage
			if err := d.Decode(&discard); err != nil {
				return nil, err
			}
			continue
		}
		if _, err = d.Token(); err != nil {
			return nil, err
		}
		for d.More() {
			var n clangNode
			if err = d.Decode(&n); err != nil {
				return nil, err
			}
			switch n.Kind {
			case "ObjCInterfaceDecl", "ObjCCategoryDecl", "ObjCProtocolDecl", "TypedefDecl", "EnumDecl", "VarDecl", "FunctionDecl", "RecordDecl":
				nodes = append(nodes, n)
			}
		}
		if _, err = d.Token(); err != nil {
			return nil, err
		}
	}
	_, err := d.Token()
	return nodes, err
}
func enumValue(n clangNode) string {
	if len(n.Value) > 0 {
		var value string
		if json.Unmarshal(n.Value, &value) == nil {
			return value
		}
		return string(n.Value)
	}
	for _, c := range n.Inner {
		if v := enumValue(c); v != "" {
			return v
		}
	}
	return ""
}
func extractMethod(owner string, n clangNode, parent sdkAttributes, source []byte, aliases map[string]string, classes map[string]bool, enums map[string]string) (sdkMethod, error) {
	a, err := attributes(n, source)
	if err != nil {
		return sdkMethod{}, err
	}
	kind := "+"
	if n.Instance {
		kind = "-"
	}
	m := sdkMethod{ID: owner + kind + n.Name, Owner: owner, Selector: n.Name, GoName: methodName(owner, n.Name), Instance: n.Instance, Implicit: n.IsImplicit, Family: methodFamily(n.Name, a.Family), ConsumesSelf: a.ConsumesSelf, Introduced: later(parent.Introduced, a.Introduced), Deprecated: earlier(parent.Deprecated, a.Deprecated), Obsoleted: earlier(parent.Obsoleted, a.Obsoleted), Unavailable: parent.Unavailable || a.Unavailable}
	m.Factory = n.Instance && m.Family == "init"
	typ := n.ReturnType
	if strings.Contains(typ.QualType, "instancetype") {
		typ.DesugaredQualType = owner + " *"
	}
	var reason string
	m.Result, reason = classifyType(typ, aliases, classes, enums, true)
	if reason != "" {
		m.Unsupported = append(m.Unsupported, "return: "+reason)
	}
	m.Ownership = "value"
	if m.Result.Kind == "object" {
		m.Ownership = "borrowed"
		if a.Retained {
			m.Ownership = "owned"
		}
		if a.NotRetained {
			m.Ownership = "borrowed"
		}
	}
	if a.ConsumesSelf && !m.Factory {
		m.Unsupported = append(m.Unsupported, "consumed receiver requires transfer policy")
	}
	if m.Instance {
		switch m.Selector {
		case "dealloc", "finalize", "retain", "release", "autorelease", "retainCount":
			m.Unsupported = append(m.Unsupported, "manual lifetime management is unavailable for managed objects")
		}
	}
	if m.Factory && m.Ownership != "owned" {
		m.Unsupported = append(m.Unsupported, "initializer lacks owned return")
	}
	if m.Unavailable {
		m.Unsupported = append(m.Unsupported, "unavailable in Objective-C on macOS")
	}
	if n.Variadic {
		m.Unsupported = append(m.Unsupported, "variadic method")
	}
	m.Unsupported = append(m.Unsupported, a.Unknown...)
	for _, p := range n.Inner {
		if p.Kind != "ParmVarDecl" {
			continue
		}
		pa, err := attributes(p, source)
		if err != nil {
			return m, err
		}
		typ, reason := classifyType(p.Type, aliases, classes, enums, false)
		if reason != "" {
			m.Unsupported = append(m.Unsupported, "parameter "+p.Name+": "+reason)
		}
		if pa.Consumed {
			m.Unsupported = append(m.Unsupported, "consumed parameter "+p.Name)
		}
		m.Unsupported = append(m.Unsupported, pa.Unknown...)
		m.Parameters = append(m.Parameters, sdkParameter{p.Name, typ, pa.Consumed})
	}
	return m, nil
}

func earlier(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	if semver.Compare("v"+a, "v"+b) < 0 {
		return a
	}
	return b
}
func mergeAttributes(a, b sdkAttributes) sdkAttributes {
	b.Introduced = later(a.Introduced, b.Introduced)
	b.Deprecated = earlier(a.Deprecated, b.Deprecated)
	b.Obsoleted = earlier(a.Obsoleted, b.Obsoleted)
	b.Unavailable = b.Unavailable || a.Unavailable
	return b
}
