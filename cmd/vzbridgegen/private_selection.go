package main

import (
	"fmt"
	"strings"
)

type privateMethodID struct {
	Class, Selector string
	ClassMethod     bool
}

var privateSelection = []privateMethodID{
	{Class: "_VZGDBDebugStubConfiguration", Selector: "initWithPort:"},
	{Class: "VZVirtualMachineConfiguration", Selector: "_setDebugStub:"},
}

func (id privateMethodID) String() string {
	kind := "-"
	if id.ClassMethod {
		kind = "+"
	}
	return fmt.Sprintf("%s[%s %s]", kind, id.Class, id.Selector)
}

func generateSelectedPrivate(metadata privateMetadata, selection []privateMethodID) ([]byte, []operation, error) {
	selected := metadata
	selected.Methods = nil
	seen := make(map[privateMethodID]bool)
	for _, id := range selection {
		if seen[id] {
			return nil, nil, fmt.Errorf("duplicate private selection %s", id)
		}
		seen[id] = true
		var match *privateMethod
		for i := range metadata.Methods {
			method := &metadata.Methods[i]
			if method.Class != id.Class || method.Selector != id.Selector || method.ClassMethod != id.ClassMethod {
				continue
			}
			if match != nil {
				return nil, nil, fmt.Errorf("multiple runtime methods match private selection %s", id)
			}
			match = method
		}
		if match == nil {
			return nil, nil, fmt.Errorf("selected private method not found: %s", id)
		}
		selected.Methods = append(selected.Methods, *match)
	}
	source, operations, unsupported, err := generatePrivate(selected)
	if err != nil {
		return nil, nil, err
	}
	if len(unsupported) != 0 {
		return nil, nil, fmt.Errorf("unsupported private selection: %s", strings.Join(unsupported, "; "))
	}
	return source, operations, nil
}
