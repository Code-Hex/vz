package main

import (
	"strings"
	"testing"
)

func TestGenerateSelectedPrivate(t *testing.T) {
	metadata := privateMetadata{Methods: []privateMethod{
		{Class: "Thing", Selector: "value", Result: "q", Arguments: []string{"@", ":"}},
		{Class: "Thing", Selector: "value", ClassMethod: true, Result: "@", Arguments: []string{"@", ":"}},
		{Class: "Other", Selector: "value", Result: "@?", Arguments: []string{"@", ":"}},
	}}
	for _, classMethod := range []bool{false, true} {
		source, operations, err := generateSelectedPrivate(metadata, []privateMethodID{{Class: "Thing", Selector: "value", ClassMethod: classMethod}})
		if err != nil {
			t.Fatal(err)
		}
		if len(operations) != 2 || operations[0].Name != "UnsafePrivateAllocate" {
			t.Fatalf("unexpected operations: %+v", operations)
		}
		if classMethod {
			if !strings.Contains(string(source), "typealias Invoke = @convention(c) (AnyClass, Selector) -> UnsafeMutableRawPointer?") || operations[1].Result != "raw" {
				t.Fatalf("class method did not preserve its runtime signature: %s", source)
			}
		} else if !strings.Contains(string(source), "typealias Invoke = @convention(c) (UnsafeMutableRawPointer?, Selector) -> Int64") || operations[1].Parameters["receiver"] != "raw" {
			t.Fatalf("instance method did not preserve its runtime signature: %s", source)
		}
	}
}

func TestGenerateSelectedPrivateErrors(t *testing.T) {
	method := privateMethod{Class: "Thing", Selector: "value", Result: "q", Arguments: []string{"@", ":"}}
	id := privateMethodID{Class: "Thing", Selector: "value"}
	for _, test := range []struct {
		name      string
		methods   []privateMethod
		selection []privateMethodID
		want      string
	}{
		{"missing", nil, []privateMethodID{id}, "selected private method not found"},
		{"wrong kind", []privateMethod{method}, []privateMethodID{{Class: "Thing", Selector: "value", ClassMethod: true}}, "selected private method not found"},
		{"duplicate selection", []privateMethod{method}, []privateMethodID{id, id}, "duplicate private selection"},
		{"duplicate discovery", []privateMethod{method, method}, []privateMethodID{id}, "multiple runtime methods match private selection"},
		{"unsupported", []privateMethod{{Class: "Thing", Selector: "value", Result: "@?", Arguments: []string{"@", ":"}}}, []privateMethodID{id}, "unsupported runtime type encoding"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := generateSelectedPrivate(privateMetadata{Methods: test.methods}, test.selection)
			if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), "Thing") || !strings.Contains(err.Error(), "value") {
				t.Fatalf("got %v, want %q with method identity", err, test.want)
			}
		})
	}
}
