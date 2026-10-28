package vmnet

import (
	"errors"
	"net"
	"runtime/cgo"
	"testing"
)

func TestInterfaceOptionNativeFlags(t *testing.T) {
	for _, tt := range []struct {
		name    string
		options []InterfaceOption
		want    uint32
	}{
		{name: "defaults"},
		{name: "disable MAC", options: []InterfaceOption{WithInterfaceMACAllocation(false)}, want: 1},
		{name: "checksum offload", options: []InterfaceOption{WithInterfaceChecksumOffload(true)}, want: 2},
		{name: "TSO", options: []InterfaceOption{WithInterfaceTSO(true)}, want: 4},
		{name: "virtio header", options: []InterfaceOption{WithInterfaceVirtioHeader(true)}, want: 16},
		{
			name: "disable header after checksum",
			options: []InterfaceOption{
				WithInterfaceVirtioHeader(true),
				WithInterfaceChecksumOffload(true),
				WithInterfaceVirtioHeader(false),
			},
			want: 2,
		},
		{name: "isolation", options: []InterfaceOption{WithInterfaceIsolation(true)}, want: 8},
		{
			name: "all capabilities",
			options: []InterfaceOption{
				WithInterfaceMACAllocation(false),
				WithInterfaceChecksumOffload(true),
				WithInterfaceTSO(true),
				WithInterfaceIsolation(true),
			},
			want: 15,
		},
		{
			name: "restore defaults",
			options: []InterfaceOption{
				WithInterfaceMACAllocation(false), WithInterfaceMACAllocation(true),
				WithInterfaceChecksumOffload(true), WithInterfaceChecksumOffload(false),
				WithInterfaceTSO(true), WithInterfaceTSO(false),
				WithInterfaceIsolation(true), WithInterfaceIsolation(false),
				WithInterfaceVirtioHeader(true), WithInterfaceVirtioHeader(false),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var c interfaceConfiguration
			for _, opt := range tt.options {
				opt(&c)
			}
			if got := c.nativeFlags(); got != tt.want {
				t.Fatalf("native flags = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestInterfaceStartupWithoutSession(t *testing.T) {
	nativeErr := errors.New("native start failed")
	for _, tt := range []struct {
		name string
		err  error
		want string
	}{
		{name: "native error", err: nativeErr, want: "native start failed"},
		{name: "missing error", want: "vmnet returned no interface"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := &interfaceResource{handle: cgo.NewHandle(make(chan struct{}, 1))}
			iface, err := completeInterfaceStart(r, 0, tt.err)
			if iface != nil || err == nil || err.Error() != tt.want {
				t.Fatalf("startup without session = %v, %v, want nil, %q", iface, err, tt.want)
			}
			if tt.err != nil && !errors.Is(err, tt.err) {
				t.Fatalf("startup error = %v, want original error %v", err, tt.err)
			}
			defer func() {
				if recover() == nil {
					r.handle.Delete()
					t.Error("startup without session retained callback handle")
				}
			}()
			r.handle.Value()
		})
	}
}

func TestInterfaceStartupRollback(t *testing.T) {
	if Available(26) {
		t.Skip("vmnet interfaces require macOS 26 and its SDK")
	}
	c, err := NewNetworkConfiguration(ModeHost)
	if err != nil {
		t.Fatal(err)
	}
	n, err := NewNetwork(c)
	if err != nil {
		t.Fatal(err)
	}
	i, err := StartInterfaceWithNetwork(n)
	if err != nil {
		t.Fatal(err)
	}
	defer i.Stop()

	failed, err := completeInterfaceStart(i.owner.resource, 0, nil)
	if err == nil || failed != nil {
		t.Fatalf("startup with invalid packet size = %v, %v", failed, err)
	}
	if i.owner.resource.session != nil {
		t.Fatal("rollback retained native session")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("rollback retained callback handle")
			}
		}()
		i.owner.resource.handle.Value()
	}()
	if err := i.WritePacket([]byte{1}); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("write after rollback = %v", err)
	}
	if err := i.Stop(); err != nil {
		t.Fatalf("stop after rollback = %v", err)
	}
	peer, err := StartInterfaceWithNetwork(n)
	if err != nil {
		t.Fatalf("reuse network after rollback = %v", err)
	}
	defer peer.Stop()
}

func TestInterfaceStopRetainsFailure(t *testing.T) {
	stopErr := errors.New("native stop failed")
	done := make(chan struct{})
	close(done)
	i := &Interface{owner: &interfaceOwner{resource: &interfaceResource{
		stopping: true,
		stopErr:  stopErr,
		done:     done,
	}}}
	copied := *i
	for _, iface := range []*Interface{i, &copied} {
		if err := iface.Stop(); !errors.Is(err, stopErr) {
			t.Fatalf("repeated stop = %v, want %v", err, stopErr)
		}
		if err := iface.WritePacket([]byte{1}); !errors.Is(err, net.ErrClosed) {
			t.Fatalf("write after failed stop = %v", err)
		}
		if _, err := iface.ReadPacket(t.Context(), nil); !errors.Is(err, net.ErrClosed) {
			t.Fatalf("read after failed stop = %v", err)
		}
	}
}
