package vz_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4"
	"golang.org/x/sys/unix"
)

var ci306CheckpointSequence atomic.Uint64

func ci306Checkpoint(t *testing.T, label string) {
	t.Helper()
	dir := os.Getenv("CI306_BARRIER_DIR")
	if dir == "" {
		return
	}
	id := ci306CheckpointSequence.Add(1)
	request, err := json.Marshal(struct {
		ID    uint64 `json:"id"`
		Label string `json:"label"`
	}{id, label})
	if err != nil {
		t.Fatalf("VZTRACE diagnostic barrier encode: %v", err)
	}
	tmp := filepath.Join(dir, fmt.Sprintf("request-%d.tmp", id))
	if err := os.WriteFile(tmp, request, 0600); err != nil {
		t.Fatalf("VZTRACE diagnostic barrier write: %v", err)
	}
	if err := os.Rename(tmp, filepath.Join(dir, "request.json")); err != nil {
		t.Fatalf("VZTRACE diagnostic barrier rename: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	want := strconv.FormatUint(id, 10)
	for time.Now().Before(deadline) {
		ack, err := os.ReadFile(filepath.Join(dir, "ack"))
		if err == nil && strings.TrimSpace(string(ack)) == want {
			return
		}
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("VZTRACE diagnostic barrier read: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("VZTRACE diagnostic barrier acknowledgement timeout: id=%d label=%s", id, label)
}

func TestRuntimeNetworkConnection(t *testing.T) {
	if vz.Available(12) {
		t.Skip("requires macOS 12")
	}
	var clientAttachments, peerAttachments []vz.NetworkDeviceAttachment
	for range 2 {
		fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM, 0)
		if err != nil {
			t.Fatal(err)
		}
		files := []*os.File{os.NewFile(uintptr(fds[0]), "client"), os.NewFile(uintptr(fds[1]), "peer")}
		for _, f := range files {
			t.Cleanup(func() { f.Close() })
		}
		client, err := vz.NewFileHandleNetworkDeviceAttachment(files[0])
		if err != nil {
			t.Fatal(err)
		}
		peer, err := vz.NewFileHandleNetworkDeviceAttachment(files[1])
		if err != nil {
			t.Fatal(err)
		}
		clientAttachments = append(clientAttachments, client)
		peerAttachments = append(peerAttachments, peer)
	}
	start := func(name string, attachments ...vz.NetworkDeviceAttachment) *Container {
		t.Helper()
		ci306Checkpoint(t, name+" start before")
		vm := newVirtualizationMachine(t, func(config *vz.VirtualMachineConfiguration) error {
			nat, err := vz.NewNATNetworkDeviceAttachment()
			if err != nil {
				return err
			}
			attachments = append([]vz.NetworkDeviceAttachment{nat}, attachments...)
			var devices []*vz.VirtioNetworkDeviceConfiguration
			for _, attachment := range attachments {
				device, err := vz.NewVirtioNetworkDeviceConfiguration(attachment)
				if err != nil {
					return err
				}
				mac, err := vz.NewRandomLocallyAdministeredMACAddress()
				if err != nil {
					return err
				}
				device.SetMACAddress(mac)
				devices = append(devices, device)
			}
			config.SetNetworkDevicesVirtualMachineConfiguration(devices)
			return nil
		})
		t.Logf("VZTRACE %s start after", name)
		t.Cleanup(func() {
			ci306Checkpoint(t, name+" shutdown before")
			defer t.Logf("VZTRACE %s shutdown after", name)
			if err := vm.Shutdown(); err != nil {
				t.Error(err)
			}
		})
		return vm
	}
	run := func(name string, vm *Container, command string) {
		t.Helper()
		ci306Checkpoint(t, name+" command before: "+command)
		defer t.Logf("VZTRACE %s command after: %s", name, command)
		if err := vm.conn.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
			t.Fatal(err)
		}
		defer vm.conn.SetDeadline(time.Time{})
		session := vm.NewSession(t)
		defer session.Close()
		output, err := session.CombinedOutput(command)
		if err != nil {
			t.Fatalf("%s: %v\n%s", command, err, output)
		}
	}
	peer := start("peer", peerAttachments...)
	run("peer", peer, "ip addr add 192.0.2.2/24 dev eth1 && ip link set eth1 up && ip addr add 198.51.100.2/24 dev eth2 && ip link set eth2 up")
	client := start("client", clientAttachments[0])
	run("client", client, "ip addr add 192.0.2.1/24 dev eth1 && ip link set eth1 up")
	run("client", client, "ping 192.0.2.2")
	ci306Checkpoint(t, "client NetworkDevices before")
	devices := client.NetworkDevices()
	t.Log("VZTRACE client NetworkDevices after")
	if len(devices) != 2 {
		t.Fatalf("got %d network devices, want 2", len(devices))
	}
	device := devices[1]
	ci306Checkpoint(t, "SetAttachment clientAttachments[1] before")
	if err := device.SetAttachment(clientAttachments[1]); err != nil {
		t.Fatal(err)
	}
	t.Log("VZTRACE SetAttachment clientAttachments[1] after")
	run("client", client, "ip addr del 192.0.2.1/24 dev eth1 && ip addr add 198.51.100.1/24 dev eth1")
	run("client", client, "ping 198.51.100.2")
	ci306Checkpoint(t, "SetAttachment nil before")
	if err := device.SetAttachment(nil); err != nil {
		t.Fatal(err)
	}
	t.Log("VZTRACE SetAttachment nil after")
	run("client", client, "if ping 198.51.100.2; then exit 1; fi")
	ci306Checkpoint(t, "SetAttachment clientAttachments[1] before")
	if err := device.SetAttachment(clientAttachments[1]); err != nil {
		t.Fatal(err)
	}
	t.Log("VZTRACE SetAttachment clientAttachments[1] after")
	run("client", client, "ping 198.51.100.2")
}
