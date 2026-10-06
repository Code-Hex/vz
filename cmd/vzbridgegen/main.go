package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	input := flag.String("input", "cmd/vzbridgegen/metadata/sdk.json", "saved generation metadata")
	extract := flag.Bool("extract", false, "refresh metadata from the current SDK and runtime without generating code")
	output := flag.String("output", "internal/vzbridge", "generated Go output directory")
	report := flag.String("report", "", "optional diagnostic JSON output path")
	flag.Parse()
	if err := run(*input, *output, *report, *extract); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(inputPath, output, reportPath string, extract bool) error {
	if extract {
		metadata, err := extractMetadata()
		if err != nil {
			return err
		}
		return writeMetadata(inputPath, metadata)
	}
	metadata, err := readMetadata(inputPath)
	if err != nil {
		return err
	}
	report := struct {
		SDK     sdkReport
		Private []privateReport
	}{SDK: metadata.SDK}
	files := map[string][]byte{}
	for _, target := range metadata.SDK.Targets {
		arch := target.Architecture
		code, err := generateSDK(target)
		if err != nil {
			return err
		}
		code = bytes.Replace(code, []byte("\n"), []byte(fmt.Sprintf("\n// SDK %s; %s.\n// Source SHA-256: %s.\n", report.SDK.SDKVersion, report.SDK.Compiler, target.SourceSHA256)), 1)
		files["sdk_generated_"+arch+".go"] = code
		contracts, err := generateContracts(target)
		if err != nil {
			return err
		}
		files["sdk_contracts_generated_"+arch+"_test.go"] = contracts
		private, privateReport, err := generatePrivate(metadata.Private, arch)
		if err != nil {
			return err
		}
		files["private_generated_"+arch+".go"] = private
		report.Private = append(report.Private, privateReport)
		supported := 0
		for _, method := range target.Methods {
			if len(method.Unsupported) == 0 {
				supported++
			}
		}
		fmt.Fprintf(os.Stderr, "%s SDK: %d generated, %d unsupported; runtime: %d generated, %d unsupported\n", arch, supported, len(target.Methods)-supported, privateReport.Generated, len(privateReport.Unsupported))
	}
	if err := os.MkdirAll(output, 0755); err != nil {
		return err
	}
	for name, code := range files {
		if err := os.WriteFile(filepath.Join(output, name), code, 0644); err != nil {
			return err
		}
	}
	if reportPath != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(reportPath, append(data, '\n'), 0644); err != nil {
			return err
		}
	}
	return nil
}
