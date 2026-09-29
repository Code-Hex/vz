package main

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var astAttribute = regexp.MustCompile(`\b(name|result|interface_type)="([^"]*)"`)
var astParameter = regexp.MustCompile(`^\(parameter "([^"]+)"`)

func inferOperations(ast io.Reader) ([]operation, error) {
	var operations []operation
	var current operation
	var resultType string
	var parameters []struct{ name, typ string }
	depth := -1
	body := false
	finish := func() error {
		if !strings.HasPrefix(current.Symbol, "vz_") {
			return nil
		}
		name := strings.TrimPrefix(current.Symbol, "vz_")
		if name == "" {
			return fmt.Errorf("empty bridge export name")
		}
		current.Name = strings.ToUpper(name[:1]) + name[1:]
		role, err := ownershipRole(resultType, true)
		if err != nil {
			return fmt.Errorf("%s result: %w", current.Symbol, err)
		}
		current.Result = role
		for _, parameter := range parameters {
			role, err := ownershipRole(parameter.typ, false)
			if err != nil {
				return fmt.Errorf("%s parameter %s: %w", current.Symbol, parameter.name, err)
			}
			if role != "" {
				if current.Parameters == nil {
					current.Parameters = make(map[string]string)
				}
				current.Parameters[parameter.name] = role
			}
		}
		operations = append(operations, current)
		return nil
	}
	scanner := bufio.NewScanner(ast)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimLeft(line, " \t")
		indent := len(line) - len(trimmed)
		if depth >= 0 && indent <= depth && trimmed != "" {
			if err := finish(); err != nil {
				return nil, err
			}
			depth = -1
		}
		if strings.HasPrefix(trimmed, "(func_decl ") {
			if depth >= 0 {
				if err := finish(); err != nil {
					return nil, err
				}
			}
			current = operation{}
			parameters = nil
			resultType = ""
			depth, body = indent, false
		}
		if depth < 0 || body {
			continue
		}
		if strings.HasPrefix(trimmed, "(brace_stmt") {
			body = true
			continue
		}
		attributes := make(map[string]string)
		for _, match := range astAttribute.FindAllStringSubmatch(trimmed, -1) {
			attributes[match[1]] = match[2]
		}
		if strings.HasPrefix(trimmed, "(cdecl_attr ") {
			current.Symbol = attributes["name"]
		}
		if typ, ok := attributes["result"]; ok {
			resultType = typ
		}
		if match := astParameter.FindStringSubmatch(trimmed); match != nil {
			parameters = append(parameters, struct{ name, typ string }{match[1], attributes["interface_type"]})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if depth >= 0 {
		if err := finish(); err != nil {
			return nil, err
		}
	}
	return operations, nil
}

func ownershipRole(typ string, result bool) (string, error) {
	switch typ {
	case "BorrowedObject":
		if !result {
			return "object", nil
		}
	case "OwnedObject":
		if result {
			return "owned", nil
		}
	case "CString":
		if !result {
			return "cstring", nil
		}
	case "ErrorOut":
		if !result {
			return "error-out", nil
		}
	case "RawPointer", "RawBytes":
		return "raw", nil
	case "()", "Void", "Bool", "Int", "UInt", "Int8", "UInt8", "Int16", "UInt16", "Int32", "UInt32", "Int64", "UInt64", "Float", "Double":
		return "", nil
	}
	return "", fmt.Errorf("unsupported Swift bridge type %q; pointers require an ownership alias", typ)
}
