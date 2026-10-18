package vmnet

import (
	"net/netip"
	"reflect"
	"testing"
)

func TestPortForwardingOptions(t *testing.T) {
	first := WithPortForwarding("tcp", 8080, netip.MustParseAddrPort("192.168.120.10:80"))
	var options, reused networkConfigurationOptions
	for _, target := range []*networkConfigurationOptions{&options, &reused} {
		if err := first(target); err != nil {
			t.Fatal(err)
		}
	}
	for _, option := range []NetworkConfigurationOption{
		WithPortForwarding("tcp", 8080, netip.MustParseAddrPort("192.168.120.11:81")),
		WithPortForwarding("udp", 8080, netip.MustParseAddrPort("192.168.120.10:82")),
		WithPortForwarding("tcp", 8080, netip.MustParseAddrPort("[fd42:1234:120::10]:83")),
	} {
		if err := option(&options); err != nil {
			t.Fatal(err)
		}
	}
	want := []portForwardingRule{
		{
			network:      "tcp",
			externalPort: 8080,
			destination:  netip.MustParseAddrPort("192.168.120.11:81"),
		},
		{
			network:      "udp",
			externalPort: 8080,
			destination:  netip.MustParseAddrPort("192.168.120.10:82"),
		},
		{
			network:      "tcp",
			externalPort: 8080,
			destination:  netip.MustParseAddrPort("[fd42:1234:120::10]:83"),
		},
	}
	if !reflect.DeepEqual(options.portForwarding, want) {
		t.Errorf("port forwarding rules = %v, want %v", options.portForwarding, want)
	}
	wantReused := []portForwardingRule{
		{
			network:      "tcp",
			externalPort: 8080,
			destination:  netip.MustParseAddrPort("192.168.120.10:80"),
		},
	}
	if !reflect.DeepEqual(reused.portForwarding, wantReused) {
		t.Errorf("reused option rules = %v, want %v", reused.portForwarding, wantReused)
	}
}
