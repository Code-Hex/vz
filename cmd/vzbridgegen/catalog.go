package main

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
)

func listFramework(public, private bool) error {
	dir, err := os.MkdirTemp("", "vzbridge-catalog-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if public {
		sdk, err := command("xcrun", "--sdk", "macosx", "--show-sdk-path")
		if err != nil {
			return err
		}
		arch := runtime.GOARCH
		if arch == "amd64" {
			arch = "x86_64"
		}
		symbols, err := discoverFramework(strings.TrimSpace(string(sdk)), arch+"-apple-macosx11.0", dir)
		if err != nil {
			return err
		}
		sort.Slice(symbols, func(i, j int) bool { return strings.Join(symbols[i].Path, ".") < strings.Join(symbols[j].Path, ".") })
		for _, symbol := range symbols {
			var declaration strings.Builder
			for _, fragment := range symbol.Fragments {
				declaration.WriteString(fragment.Spelling)
			}
			fmt.Printf("%s\t%s\t%s\n", symbol.Kind.Identifier, strings.Join(symbol.Path, "."), declaration.String())
		}
	}
	if private {
		metadata, err := discoverPrivate(dir, nil)
		if err != nil {
			return err
		}
		for _, method := range metadata.Methods {
			kind := "-"
			if method.ClassMethod {
				kind = "+"
			}
			fmt.Printf("%s[%s %s]\t%s (%s)\n", kind, method.Class, method.Selector, method.Result, strings.Join(method.Arguments, ", "))
		}
	}
	return nil
}
