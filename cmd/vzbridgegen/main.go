package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	output := flag.String("output", "internal/vzbridge", "generated Go output directory")
	report := flag.String("report", "", "optional diagnostic JSON output path")
	flag.Parse()
	if err := run(*output, *report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(output, reportPath string) error {
	directory, err := os.MkdirTemp("", "vz-sdk-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(directory)
	input := filepath.Join(directory, "scan.m")
	if err = os.WriteFile(input, []byte("#import <Virtualization/Virtualization.h>\n"), 0600); err != nil {
		return err
	}
	version, err := exec.Command("xcrun", "--show-sdk-version").Output()
	if err != nil {
		return err
	}
	compiler, err := exec.Command("xcrun", "clang", "--version").Output()
	if err != nil {
		return err
	}
	report := struct {
		SDK     sdkReport
		Private []privateReport
	}{SDK: sdkReport{Schema: 1, SDKVersion: strings.TrimSpace(string(version)), Compiler: strings.Split(string(compiler), "\n")[0]}}
	metadata, err := discoverPrivate(directory)
	if err != nil {
		return err
	}
	files := map[string][]byte{}
	for _, arch := range []string{"arm64", "amd64"} {
		targetArch := arch
		if arch == "amd64" {
			targetArch = "x86_64"
		}
		target, err := extractTarget(input, arch, targetArch+"-apple-macos11", directory)
		if err != nil {
			return err
		}
		report.SDK.Targets = append(report.SDK.Targets, target)
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
		private, privateReport, err := generatePrivate(metadata, arch)
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
