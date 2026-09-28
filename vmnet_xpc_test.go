package vz_test

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"net/netip"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"text/template"

	"github.com/Code-Hex/vz/v3/internal/osversion"
	"github.com/Code-Hex/vz/v3/vmnet"
	"github.com/Code-Hex/vz/v3/xpc"
)

var vmnetXPCServer = flag.Bool("vmnet-xpc-server", false, "run vmnet XPC test service")
var vmnetXPCSubnet = flag.String("vmnet-xpc-subnet", "", "subnet for vmnet XPC test service")
var vmnetXPCService = flag.String("vmnet-xpc-service", "", "Mach service for vmnet XPC test")

// TestVmnetNetworkSharingOverXPC verifies that a serialized network can be used
// by virtual machines in separate processes.
func TestVmnetNetworkSharingOverXPC(t *testing.T) {
	if err := osversion.MacOSAvailable(26); err != nil {
		t.Skipf("vmnet network requires macOS 26: %v", err)
	}

	label := fmt.Sprintf("dev.code-hex.vz.test.vmnet-xpc.%d", os.Getpid())
	service := label + ".network"
	if *vmnetXPCServer {
		runVmnetXPCServer(t, *vmnetXPCService)
		return
	}

	subnet := detectFreeIPv4Subnet(t, netip.MustParsePrefix("192.168.6.0/24"))
	registerVmnetXPCService(t, label, service, subnet)

	session, err := xpc.NewSession(service)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Cancel()
	response, err := session.SendDictionaryWithReply(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if message := response.GetString("Error"); message != "" {
		t.Fatal(message)
	}
	serialization := response.GetValue("Serialization")
	if serialization == nil {
		t.Fatal("XPC response has no network serialization")
	}
	network, err := vmnet.NewNetworkWithSerialization(serialization)
	if err != nil {
		t.Fatal(err)
	}
	container := newVirtualizationMachine(t, configureNetworkDevice(network, randomMACAddress(t)))
	t.Cleanup(func() {
		if err := container.Shutdown(); err != nil {
			t.Error(err)
		}
	})
	address := netip.MustParseAddr(container.DetectIPv4(t, "eth0"))
	if !subnet.Contains(address) {
		t.Fatalf("client VM address %s is outside %s", address, subnet)
	}
}

func runVmnetXPCServer(t *testing.T, service string) {
	subnet, err := netip.ParsePrefix(*vmnetXPCSubnet)
	if err != nil {
		t.Fatal(err)
	}
	config, err := vmnet.NewNetworkConfiguration(vmnet.SharedMode)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.SetIPv4Subnet(subnet); err != nil {
		t.Fatal(err)
	}
	network, err := vmnet.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	serialization, err := network.CopySerialization()
	if err != nil {
		t.Fatal(err)
	}
	container := newVirtualizationMachine(t, configureNetworkDevice(network, randomMACAddress(t)))
	t.Cleanup(func() {
		if err := container.Shutdown(); err != nil {
			t.Error(err)
		}
	})
	address := netip.MustParseAddr(container.DetectIPv4(t, "eth0"))
	if !subnet.Contains(address) {
		t.Fatalf("server VM address %s is outside %s", address, subnet)
	}

	listener, err := xpc.NewListener(service, xpc.Accept(xpc.MessageHandler(func(request *xpc.Dictionary) *xpc.Dictionary {
		return request.CreateReply(xpc.KeyValue("Serialization", serialization))
	})))
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Activate(); err != nil {
		t.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(t.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	if err := listener.Close(); err != nil {
		t.Error(err)
	}
}

const vmnetXPCPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>{{.Label}}</string>
<key>ProgramArguments</key><array>{{range .Arguments}}<string>{{.}}</string>{{end}}</array>
<key>RunAtLoad</key><true/>
<key>WorkingDirectory</key><string>{{.WorkingDirectory}}</string>
<key>StandardErrorPath</key><string>{{.StandardErrorPath}}</string>
<key>MachServices</key><dict><key>{{.Service}}</key><true/></dict>
</dict></plist>`

func registerVmnetXPCService(t *testing.T, label, service string, subnet netip.Prefix) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "Library", "LaunchAgents", label+".plist")
	stderrPath := filepath.Join(t.TempDir(), "vmnet-xpc-stderr.log")
	var plist bytes.Buffer
	err = template.Must(template.New("launchd").Parse(vmnetXPCPlist)).Execute(&plist, struct {
		Label, Service, WorkingDirectory, StandardErrorPath string
		Arguments                                           []string
	}{label, service, cwd, stderrPath, []string{os.Args[0], "-test.run=^TestVmnetNetworkSharingOverXPC$", "-vmnet-xpc-server", "-vmnet-xpc-subnet=" + subnet.String(), "-vmnet-xpc-service=" + service}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, plist.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Remove(path); err != nil {
			t.Error(err)
		}
	})
	if output, err := exec.CommandContext(t.Context(), "launchctl", "load", path).CombinedOutput(); err != nil {
		t.Fatalf("launchctl load: %v: %s", err, output)
	}
	t.Cleanup(func() {
		if output, err := exec.CommandContext(context.Background(), "launchctl", "unload", path).CombinedOutput(); err != nil {
			t.Errorf("launchctl unload: %v: %s", err, output)
		}
		if stderr, err := os.ReadFile(stderrPath); err == nil {
			t.Logf("XPC service output:\n%s", stderr)
		}
	})
	log.Printf("registered vmnet XPC service %s for %s", service, subnet)
}
