package vmnet_test

import (
	"context"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4/internal/vzbridge"
	"github.com/Code-Hex/vz/v4/vmnet"
)

func TestNetworkSharingInvalidInputs(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	for _, input := range []struct {
		name        string
		requirement string
	}{
		{
			name:        "",
			requirement: "true",
		},
		{
			name:        "test\x00name",
			requirement: "true",
		},
		{
			name:        "test",
			requirement: "",
		},
		{
			name:        "test",
			requirement: "true\x00false",
		},
	} {
		if _, err := vmnet.ReceiveNetwork(context.Background(), input.name, input.requirement); err == nil {
			t.Fatal("invalid service arguments accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := vmnet.ReceiveNetwork(ctx, "com.codehex.vz.test.absent", "true"); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want canceled", err)
	}
}

func TestNetworkServiceProcess(t *testing.T) {
	args := flag.Args()
	if len(args) == 0 || args[0] != "vmnet-network-service" {
		return
	}
	if len(args) != 4 {
		t.Fatal("network service helper requires a name, signing requirement, and ready path")
	}
	name, requirement, ready := args[1], args[2], args[3]
	stopped := make(chan os.Signal, 1)
	signal.Notify(stopped, syscall.SIGTERM)
	defer signal.Stop(stopped)
	wait := func() {
		select {
		case <-stopped:
		case <-time.After(30 * time.Second):
		}
	}
	if requirement == "stall" {
		if err := writeNetworkServiceReady(ready); err != nil {
			t.Fatal(err)
		}
		wait()
		return
	}
	config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.SetIPv4Subnet(netip.MustParsePrefix("192.168.248.0/24")); err != nil {
		t.Fatal(err)
	}
	network, err := vmnet.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	service, err := vmnet.ListenNetwork(name, network, requirement)
	if err != nil {
		t.Fatal(err)
	}
	network = nil
	for range 10 {
		runtime.GC()
		vzbridge.Drain()
		time.Sleep(10 * time.Millisecond)
	}
	alias := *service
	defer service.Close()
	if err := writeNetworkServiceReady(ready); err != nil {
		t.Fatal(err)
	}
	wait()
	var closeGroup sync.WaitGroup
	for _, copy := range []*vmnet.NetworkService{service, &alias} {
		closeGroup.Add(1)
		go func() {
			defer closeGroup.Done()
			if err := copy.Close(); err != nil {
				t.Error(err)
			}
		}()
	}
	closeGroup.Wait()
	if !t.Failed() {
		if err := os.WriteFile(ready+".closed", nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func writeNetworkServiceReady(path string) error {
	if err := os.WriteFile(path+".tmp", []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

func TestNetworkSharingProcesses(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	requirement := networkTestRequirement(t)
	for _, test := range []struct {
		name              string
		serverRequirement string
		clientRequirement string
		success           bool
	}{
		{
			name:              "roundtrip",
			serverRequirement: requirement,
			clientRequirement: requirement,
			success:           true,
		},
		{
			name:              "rejectServer",
			serverRequirement: requirement,
			clientRequirement: "false",
		},
		{
			name:              "rejectClient",
			serverRequirement: "false",
			clientRequirement: requirement,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := startNetworkService(t, test.serverRequirement)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			network, err := vmnet.ReceiveNetwork(ctx, service.name, test.clientRequirement)
			if !test.success {
				if err == nil {
					t.Fatal("unauthorized peer accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			iface, err := vmnet.StartInterfaceWithNetwork(network)
			if err != nil {
				t.Fatal(err)
			}
			if err := iface.Stop(); err != nil {
				t.Fatal(err)
			}
			got, err := network.IPv4Subnet()
			if err != nil {
				t.Fatal(err)
			}
			want := netip.MustParsePrefix("192.168.248.0/24")
			if got != want {
				t.Fatalf("subnet = %v, want %v", got, want)
			}
		})
	}
}

type networkServiceProcess struct {
	name   string
	pid    int
	killed bool
}

func (p *networkServiceProcess) kill(t *testing.T) {
	t.Helper()
	if err := syscall.Kill(p.pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	p.killed = true
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if errors.Is(syscall.Kill(p.pid, 0), syscall.ESRCH) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("network service process %d did not exit after SIGKILL", p.pid)
}

func startNetworkService(t *testing.T, requirement string) *networkServiceProcess {
	t.Helper()
	dir := t.TempDir()
	name := fmt.Sprintf("com.codehex.vz.test.network.%d.%d", os.Getpid(), time.Now().UnixNano())
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(dir, "ready")
	escape := func(s string) string {
		var b strings.Builder
		xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key>
<array>
<string>%s</string>
<string>-test.run=^TestNetworkServiceProcess$</string>
<string>--</string>
<string>vmnet-network-service</string>
<string>%s</string>
<string>%s</string>
<string>%s</string>
</array>
<key>MachServices</key><dict><key>%s</key><true/></dict>
<key>RunAtLoad</key><true/>
</dict></plist>`, name, escape(executable), name, escape(requirement), escape(ready), name)
	path := filepath.Join(dir, "service.plist")
	if err := os.WriteFile(path, []byte(plist), 0600); err != nil {
		t.Fatal(err)
	}
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	if output, err := exec.Command("launchctl", "bootstrap", domain, path).CombinedOutput(); err != nil {
		t.Fatalf("bootstrap: %v: %s", err, output)
	}
	process := &networkServiceProcess{
		name: name,
	}
	t.Cleanup(func() {
		if output, err := exec.Command("launchctl", "bootout", domain+"/"+name).CombinedOutput(); err != nil {
			t.Errorf("bootout: %v: %s", err, output)
		}
		data, err := os.ReadFile(ready)
		if err != nil {
			return
		}
		pid, err := strconv.Atoi(string(data))
		if err != nil {
			t.Error(err)
			return
		}
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
				if requirement != "stall" && !process.killed {
					if _, err := os.Stat(ready + ".closed"); err != nil {
						t.Errorf("network service did not finish Close: %v", err)
					}
				}
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Errorf("network service process %d did not exit", pid)
	})
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(ready); err == nil {
			pid, err := strconv.Atoi(string(data))
			if err != nil {
				t.Fatal(err)
			}
			process.pid = pid
			return process
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("network service did not start")
	return nil
}

func TestNetworkSharingInvalidRequirement(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	if _, err := vmnet.ReceiveNetwork(context.Background(), "com.codehex.vz.test", "not a valid requirement ("); err == nil {
		t.Fatal("invalid requirement accepted")
	}
	config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
	if err != nil {
		t.Fatal(err)
	}
	network, err := vmnet.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vmnet.ListenNetwork("com.codehex.vz.test", network, "not a valid requirement ("); err == nil {
		t.Fatal("invalid requirement accepted")
	}
}

func TestNetworkSharingCancellation(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	service := startNetworkService(t, "stall")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := vmnet.ReceiveNetwork(ctx, service.name, "true")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
}

func TestNetworkSharingMetadataAfterServerExit(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	requirement := networkTestRequirement(t)
	service := startNetworkService(t, requirement)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	network, err := vmnet.ReceiveNetwork(ctx, service.name, requirement)
	if err != nil {
		t.Fatal(err)
	}
	service.kill(t)
	got, err := network.IPv4Subnet()
	if err != nil {
		t.Fatal(err)
	}
	want := netip.MustParsePrefix("192.168.248.0/24")
	if got != want {
		t.Fatalf("subnet after server exit = %v, want %v", got, want)
	}
	serialization, err := network.CopySerialization()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := vmnet.NewNetworkWithSerialization(serialization)
	if err != nil {
		t.Fatal(err)
	}
	got, err = restored.IPv4Subnet()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("restored subnet after server exit = %v, want %v", got, want)
	}
}

func TestNetworkSharingServerExitDuringRequest(t *testing.T) {
	if vmnet.Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	requirement := networkTestRequirement(t)
	service := startNetworkService(t, requirement)
	if err := syscall.Kill(service.pid, syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !service.killed {
			syscall.Kill(service.pid, syscall.SIGCONT)
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for {
		output, err := exec.Command("ps", "-o", "state=", "-p", strconv.Itoa(service.pid)).Output()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(output), "T") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("network service did not stop")
		}
		time.Sleep(20 * time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := vmnet.ReceiveNetwork(ctx, service.name, requirement)
		result <- err
	}()
	select {
	case err := <-result:
		t.Fatalf("request completed while server was stopped: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	service.kill(t)
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("request succeeded after server exit")
		}
		if errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("request waited for its deadline after server exit: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("request did not complete after server exit")
	}
}

func networkTestRequirement(t *testing.T) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("codesign", "-d", "-vvvv", executable).CombinedOutput()
	if err != nil {
		t.Fatalf("codesign: %v: %s", err, output)
	}
	for _, line := range strings.Split(string(output), "\n") {
		if hash, found := strings.CutPrefix(line, "CDHash="); found {
			return `cdhash H"` + hash + `"`
		}
	}
	t.Fatal("codesign returned no CDHash")
	return ""
}
