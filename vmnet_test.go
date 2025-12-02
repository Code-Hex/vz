package vz_test

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"text/template"

	"github.com/Code-Hex/vz/v3"
	"github.com/Code-Hex/vz/v3/vmnet"
	"github.com/Code-Hex/vz/v3/xpc"
)

// TestVmnetSharedModeAllowsCommunicationBetweenMultipleVMs tests VmnetNetwork in SharedMode
// allows communication between multiple VMs connected to the same VmnetNetwork instance.
// This test creates two VmnetNetwork instances by serializing and deserializing the first instance,
// then boots a VM using each VmnetNetwork instance and tests communication between the two VMs.
func TestVmnetSharedModeAllowsCommunicationBetweenMultipleVMs(t *testing.T) {
	if vz.Available(26.0) {
		t.Skip("VmnetSharedMode is supported from macOS 26")
	}

	// Create VmnetNetwork instance from configuration
	config, err := vmnet.NewNetworkConfiguration(vmnet.SharedMode)
	if err != nil {
		t.Fatal(err)
	}
	network1, err := vmnet.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}
	macaddress1 := randomMACAddress(t)

	// Create another VmnetNetwork instance from serialization of the first one
	serialization, err := network1.CopySerialization()
	if err != nil {
		t.Fatal(err)
	}
	network2, err := vmnet.NewNetworkWithSerialization(serialization)
	if err != nil {
		t.Fatal(err)
	}
	macaddress2 := randomMACAddress(t)

	container1 := newVirtualizationMachine(t, configureNetworkDevice(network1, macaddress1))
	container2 := newVirtualizationMachine(t, configureNetworkDevice(network2, macaddress2))
	t.Cleanup(func() {
		if err := container1.Shutdown(); err != nil {
			log.Println(err)
		}
		if err := container2.Shutdown(); err != nil {
			log.Println(err)
		}
	})

	// Log network information
	ipv4Subnet, err := network1.IPv4Subnet()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Vmnet network IPv4 subnet: %s", ipv4Subnet.String())
	prefix, err := network1.IPv6Prefix()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Vmnet network IPv6 prefix: %s", prefix.String())

	// Detect IP addresses and test communication between VMs
	container1IPv4 := container1.DetectIPv4(t, "eth0")
	t.Logf("Container 1 IPv4: %s", container1IPv4)
	container2IPv4 := container2.DetectIPv4(t, "eth0")
	t.Logf("Container 2 IPv4: %s", container2IPv4)
	container1.exec(t, "ping "+container2IPv4)
	container2.exec(t, "ping "+container1IPv4)
}

// TestVmnetSharedModeWithConfiguringIPv4 tests VmnetNetwork in SharedMode
// with custom IPv4 subnet and DHCP reservation.
// This test creates a VmnetNetwork instance with a specified IPv4 subnet and DHCP reservation,
// then boots a VM using the VmnetNetwork and verifies the VM receives the expected IP address.
func TestVmnetSharedModeWithConfiguringIPv4(t *testing.T) {
	if vz.Available(26.0) {
		t.Skip("VmnetSharedMode is supported from macOS 26")
	}
	// Create VmnetNetwork instance from configuration
	config, err := vmnet.NewNetworkConfiguration(vmnet.SharedMode)
	if err != nil {
		t.Fatal(err)
	}
	// Configure IPv4 subnet
	ipv4Subnet := detectFreeIPv4Subnet(t, netip.MustParsePrefix("192.168.5.0/24"))
	if err := config.SetIPv4Subnet(ipv4Subnet); err != nil {
		t.Fatal(err)
	}
	// Configure DHCP reservation
	macaddress := randomMACAddress(t)
	ipv4 := netip.MustParseAddr("192.168.5.15")
	if err := config.AddDhcpReservation(macaddress.HardwareAddr(), ipv4); err != nil {
		t.Fatal(err)
	}

	// Create VmnetNetwork instance
	network, err := vmnet.NewNetwork(config)
	if err != nil {
		t.Fatal(err)
	}

	// Create VirtualizationMachine instance
	container := newVirtualizationMachine(t, configureNetworkDevice(network, macaddress))
	t.Cleanup(func() {
		if err := container.Shutdown(); err != nil {
			log.Println(err)
		}
	})

	// Log network information
	ipv4SubnetConfigured, err := network.IPv4Subnet()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Vmnet network IPv4 subnet: %s", ipv4SubnetConfigured.String())

	// Verify the configured subnet
	// Compare with masked value to ignore host bits since Vmnet prefers to use first address as network address.
	if ipv4Subnet != ipv4SubnetConfigured.Masked() {
		t.Fatalf("expected IPv4 subnet %s, but got %s", ipv4Subnet.String(), ipv4SubnetConfigured.Masked().String())
	}

	// Log IPv6 prefix
	prefix, err := network.IPv6Prefix()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Vmnet network IPv6 prefix: %s", prefix.String())

	// Detect IP address and verify DHCP reservation
	containerIPv4 := container.DetectIPv4(t, "eth0")
	t.Logf("Container IPv4: %s", containerIPv4)
	if ipv4.String() != containerIPv4 {
		t.Fatalf("expected IPv4 %s, but got %s", ipv4, containerIPv4)
	}
}

var server bool

func init() {
	// Determine if running as server or client based on command-line arguments
	flag.BoolVar(&server, "server", false, "run as mach service server")
}

// TestVmnetNetworkShareModeSharingOverXpc tests sharing VmnetNetwork in SharedMode over XPC communication.
// This test registers test executable as an Mach service and launches it using launchctl.
// The launched Mach service provides VmnetNetwork serialization to clients upon request, after booting
// a VM using the provided VmnetNetwork to ensure the network is functional on the server side.
// The client boots VM using the provided VmnetNetwork serialization.
//
// This test uses pkg/xpc package to implement XPC communication.
func TestVmnetNetworkShareModeSharingOverXpc(t *testing.T) {
	if vz.Available(26.0) {
		t.Skip("VmnetSharedMode is supported from macOS 26")
	}

	label := "dev.code-hex.vz.test.vmnetsharedmode"
	machServiceName := label + ".subnet"

	if server {
		t.Log("running as mach service server")
		listener, err := xpcServerProvidingSubnet(t, machServiceName)
		if err != nil {
			log.Printf("failed to create mach service server: %v", err)
			t.Fatal(err)
		}
		if err := listener.Activate(); err != nil {
			log.Printf("failed to activate mach service server: %v", err)
			t.Fatal(err)
		}
		ctx, stop := signal.NotifyContext(t.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		<-ctx.Done()
		if err := listener.Close(); err != nil {
			log.Printf("failed to close mach service server: %v", err)
		}
	} else {
		t.Log("running as mach service client")
		xpcRegisterMachService(t, label, machServiceName)
		ipv4Subnet := detectFreeIPv4Subnet(t, netip.MustParsePrefix("192.168.6.0/24"))
		network, err := xpcClientRequestingSubnet(t, machServiceName, ipv4Subnet)
		if err != nil {
			t.Fatal(err)
		}
		container := newVirtualizationMachine(t, configureNetworkDevice(network, randomMACAddress(t)))
		t.Cleanup(func() {
			if err := container.Shutdown(); err != nil {
				log.Println(err)
			}
		})
		containerIPv4 := container.DetectIPv4(t, "eth0")
		t.Logf("Container IPv4: %s", containerIPv4)
		if !ipv4Subnet.Contains(netip.MustParseAddr(containerIPv4)) {
			t.Fatalf("expected container IPv4 %s to be within subnet %s", containerIPv4, ipv4Subnet)
		}
	}
}

// configureNetworkDevice returns a function that configures a network device
// with the given VmnetNetwork and MAC address.
func configureNetworkDevice(network *vmnet.Network, macAddress *vz.MACAddress) func(cfg *vz.VirtualMachineConfiguration) error {
	return func(cfg *vz.VirtualMachineConfiguration) error {
		var configurations []*vz.VirtioNetworkDeviceConfiguration
		attachment, err := vz.NewVmnetNetworkDeviceAttachment(network.Raw())
		if err != nil {
			return err
		}
		config, err := vz.NewVirtioNetworkDeviceConfiguration(attachment)
		if err != nil {
			return err
		}
		config.SetMACAddress(macAddress)
		configurations = append(configurations, config)
		cfg.SetNetworkDevicesVirtualMachineConfiguration(configurations)
		return nil
	}
}

// detectFreeIPv4Subnet detects a free IPv4 subnet on the host machine.
func detectFreeIPv4Subnet(t *testing.T, prefer netip.Prefix) netip.Prefix {
	targetPrefix := netip.MustParsePrefix("192.168.0.0/16")
	hostNetIfs, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	candidates := make([]netip.Prefix, len(hostNetIfs))
	for _, hostNetIf := range hostNetIfs {
		hostNetAddrs, err := hostNetIf.Addrs()
		if err != nil {
			continue
		}
		for _, hostNetAddr := range hostNetAddrs {
			netIPNet, ok := hostNetAddr.(*net.IPNet)
			if !ok {
				continue
			}
			hostPrefix := netip.MustParsePrefix(netIPNet.String())
			if targetPrefix.Overlaps(hostPrefix) {
				candidates = append(candidates, hostPrefix)
			}
		}
	}
	slices.SortFunc(candidates, func(l, r netip.Prefix) int {
		if l.Addr().Less(r.Addr()) {
			return -1
		}
		return 1
	})
	for _, candidate := range candidates {
		if prefer.Addr() != candidate.Addr() {
			return prefer
		}
	}
	t.Fatal("no free IPv4 subnet found")
	return netip.Prefix{}
}

// funcName returns the name of the calling function.
// It is used to get the test function name for launchctl registration.
func funcName(t *testing.T, skip int) string {
	pc, _, _, ok := runtime.Caller(skip)
	if !ok {
		t.Fatal("failed to get caller info")
	}
	funcNameComponents := strings.Split(runtime.FuncForPC(pc).Name(), ".")
	return funcNameComponents[len(funcNameComponents)-1]
}

// randomMACAddress generates a random locally administered MAC address.
func randomMACAddress(t *testing.T) *vz.MACAddress {
	mac, err := vz.NewRandomLocallyAdministeredMACAddress()
	if err != nil {
		t.Fatal(err)
	}
	return mac
}

// xpcClientRequestingSubnet requests a VmnetNetwork serialization for the given subnet
// from the Mach service and returns the deserialized VmnetNetwork instance.
func xpcClientRequestingSubnet(t *testing.T, machServiceName string, subnet netip.Prefix) (*vmnet.Network, error) {
	session, err := xpc.NewSession(machServiceName)
	if err != nil {
		return nil, err
	}
	defer session.Cancel()

	resp, err := session.SendDictionaryWithReply(
		t.Context(),
		xpc.KeyValue("Subnet", xpc.NewString(subnet.String())),
	)
	if err != nil {
		return nil, err
	}
	errorStr := resp.GetString("Error")
	if errorStr != "" {
		return nil, fmt.Errorf("xpc service error: %s", errorStr)
	}
	serialization := resp.GetValue("Serialization")
	log.Printf("%v", serialization)
	if serializationDic, ok := serialization.(*xpc.Dictionary); ok {
		serializationData := serializationDic.GetData("networkSerialization")
		fmt.Printf("serialization data: %q\n", hex.EncodeToString(serializationData))
	}
	return vmnet.NewNetworkWithSerialization(serialization.Raw())
}

const launchdPlistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
    <dict>
        <key>Label</key>
        <string>{{.Label}}</string>
        <key>ProgramArguments</key>
        <array>
            {{- range $arg := .ProgramArguments}}
            <string>{{$arg}}</string>
            {{- end}}
        </array>
        <key>RunAtLoad</key>
        <true/>
        <key>WorkingDirectory</key>
        <string>{{ .WorkingDirectory }}</string>
        <key>StandardErrorPath</key>
        <string>{{ .WorkingDirectory }}/vmnet_test.xpc_test.stderr.log</string>
        <!-- <key>StandardOutPath</key>
        <string>{{ .WorkingDirectory }}/vmnet_test.xpc_test.stdout.log</string> -->
        <key>MachServices</key>
        <dict>
            {{- range $service := .MachServices}}
            <key>{{$service}}</key>
            <true/>
            {{- end}}
        </dict>
    </dict>
</plist>`

// xpcRegisterMachService registers the test executable as a Mach service
// using launchctl with the given label and machServiceName.
// The launched Mach service stderr output will be redirected to the ./vmnet_test.xpc_test.stderr.log file.
func xpcRegisterMachService(t *testing.T, label, machServiceName string) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	params := struct {
		Label            string
		ProgramArguments []string
		WorkingDirectory string
		MachServices     []string
	}{
		Label:            label,
		ProgramArguments: []string{os.Args[0], "-test.run", "^" + funcName(t, 2) + "$", "-server"},
		WorkingDirectory: cwd,
		MachServices:     []string{machServiceName},
	}
	template, err := template.New("plist").Parse(launchdPlistTemplate)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := template.Execute(&b, params); err != nil {
		t.Fatal(err)
	}
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	launchAgentDir := path.Join(userHomeDir, "Library", "LaunchAgents", label+".plist")
	if err := os.WriteFile(launchAgentDir, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Remove(launchAgentDir); err != nil {
			t.Logf("failed to remove launch agent plist: %v", err)
		}
	})
	cmd := exec.CommandContext(t.Context(), "launchctl", "load", launchAgentDir)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// do not use t.Context() here to ensure unload runs
		cmd := exec.CommandContext(context.Background(), "launchctl", "unload", launchAgentDir)
		if err := cmd.Run(); err != nil {
			t.Logf("failed to unload launch agent: %v", err)
		}
	})
}

// xpcServerProvidingSubnet creates an Mach service XPC listener
// that provides VmnetNetwork serialization for requested subnet.
// It also boots a VM using the provided VmnetNetwork to ensure the network is functional on the server side.
func xpcServerProvidingSubnet(t *testing.T, machServiceName string) (*xpc.Listener, error) {
	return xpc.NewListener(
		machServiceName,
		xpc.Accept(
			xpc.MessageHandler(func(dic *xpc.Dictionary) *xpc.Dictionary {
				createErrorReply := func(errMsg string, args ...any) *xpc.Dictionary {
					errorString := fmt.Sprintf(errMsg, args...)
					log.Print(errorString)
					t.Log(errorString)
					return dic.CreateReply(
						xpc.KeyValue("Error", xpc.NewString(errorString)),
					)
				}
				var reply *xpc.Dictionary
				if subnet := dic.GetString("Subnet"); subnet == "" {
					reply = createErrorReply("missing Subnet in request")
				} else if config, err := vmnet.NewNetworkConfiguration(vmnet.SharedMode); err != nil {
					reply = createErrorReply("failed to create vmnet network configuration: %v", err)
				} else if err := config.SetIPv4Subnet(netip.MustParsePrefix(subnet)); err != nil {
					reply = createErrorReply("failed to set ipv4 subnet: %v", err)
				} else if network, err := vmnet.NewNetwork(config); err != nil {
					reply = createErrorReply("failed to create vmnet network: %v", err)
				} else if serialization, err := network.CopySerialization(); err != nil {
					reply = createErrorReply("failed to copy serialization: %v", err)
				} else {
					container := newVirtualizationMachine(t, configureNetworkDevice(network, randomMACAddress(t)))
					t.Cleanup(func() {
						if err := container.Shutdown(); err != nil {
							log.Println(err)
						}
					})
					containerIPv4 := container.DetectIPv4(t, "eth0")
					log.Printf("Container IPv4: %s", containerIPv4)
					t.Logf("Container IPv4: %s", containerIPv4)
					if netip.MustParsePrefix(subnet).Contains(netip.MustParseAddr(containerIPv4)) {
						reply = dic.CreateReply(
							xpc.KeyValue("Serialization", xpc.NewObject(serialization)),
						)
					} else {
						reply = createErrorReply("allocated container IPv4 %s is not within requested subnet %s", containerIPv4, subnet)
					}
				}
				return reply
			}),
		),
	)
}
