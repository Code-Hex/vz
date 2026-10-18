package vmnet

import (
	"errors"
	"net"
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
