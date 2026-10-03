//go:build darwin

package vzbridge

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnusedBindingsAreNotLinked(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": fmt.Sprintf("module bindingconsumer\n\ngo 1.25.0\n\nrequire github.com/Code-Hex/vz/v4 v4.0.0\nreplace github.com/Code-Hex/vz/v4 => %q\n", root),
		"main.go": `package main
import (
 "fmt"
 vz "github.com/Code-Hex/vz/v4"
)
func main() { fmt.Println(vz.VirtualMachineConfigurationMinimumAllowedCPUCount()) }
`,
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	binary := filepath.Join(dir, "consumer")
	build := exec.Command("go", "build", "-mod=mod", "-o", binary, ".")
	build.Dir = dir
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build consumer: %v\n%s", err, out)
	}
	if out, err := exec.Command(binary).CombinedOutput(); err != nil || string(out) != "1\n" {
		t.Fatalf("consumer result = %q, error = %v", out, err)
	}
	out, err := exec.Command("go", "tool", "nm", binary).CombinedOutput()
	if err != nil {
		t.Fatalf("inspect consumer: %v\n%s", err, out)
	}
	const bridge = "github.com/Code-Hex/vz/v4/internal/vzbridge."
	const used = bridge + "sdkCallVZVirtualMachineConfiguration_MinimumAllowedCPUCount"
	foundUsed := false
	var unused []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		symbol := strings.Join(fields[2:], " ")
		if symbol == used {
			foundUsed = true
			continue
		}
		if strings.HasPrefix(symbol, bridge+"sdkCall") ||
			strings.HasPrefix(symbol, bridge+"privateCall") ||
			strings.HasPrefix(symbol, bridge+"UnsafePrivate") ||
			strings.HasPrefix(symbol, bridge+"call") ||
			strings.HasPrefix(symbol, "_vz_") && symbol != "_vz_dispatchSync" {
			unused = append(unused, symbol)
		}
	}
	if len(unused) != 0 {
		t.Errorf("%d unused bindings remain linked; first symbols: %v", len(unused), unused[:min(len(unused), 10)])
	}
	if !foundUsed {
		t.Fatal("used SDK binding is missing from symbol inventory")
	}
}
