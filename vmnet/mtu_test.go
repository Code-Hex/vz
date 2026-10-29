package vmnet

import (
	"math"
	"strings"
	"testing"
)

func TestWithMTU(t *testing.T) {
	for _, test := range []struct {
		mtu   int
		valid bool
	}{
		{
			mtu: math.MinInt,
		},
		{
			mtu: -1,
		},
		{
			mtu: 0,
		},
		{
			mtu: 72,
		},
		{
			mtu: 1279,
		},
		{
			mtu:   1280,
			valid: true,
		},
		{
			mtu:   1500,
			valid: true,
		},
		{
			mtu:   9000,
			valid: true,
		},
		{
			mtu:   65535,
			valid: true,
		},
		{
			mtu: 65536,
		},
		{
			mtu: math.MaxInt,
		},
	} {
		var options networkConfigurationOptions
		err := WithMTU(test.mtu)(&options)
		if (err == nil) != test.valid {
			t.Errorf("WithMTU(%d) = %v, want valid %v", test.mtu, err, test.valid)
		}
		if test.valid && options.mtu != test.mtu {
			t.Errorf("MTU = %d, want %d", options.mtu, test.mtu)
		}
		if !test.valid && options.mtu != 0 {
			t.Errorf("invalid MTU changed options to %d", options.mtu)
		}
	}
}

func TestNetworkConfigurationInvalidMTU(t *testing.T) {
	for _, mtu := range []int{-1, 0, 1279, 65536, math.MaxInt} {
		// An invalid mode also prevents native allocation if option validation regresses.
		config, err := NewNetworkConfiguration(Mode(-1), WithMTU(mtu))
		if config != nil || err == nil || !strings.Contains(err.Error(), "invalid vmnet MTU") {
			t.Errorf("WithMTU(%d): configuration = %v, error = %v", mtu, config, err)
		}
	}
}

func TestWithMTUInterface(t *testing.T) {
	if Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	for _, mode := range []Mode{ModeHost, ModeShared} {
		config, err := NewNetworkConfiguration(mode, WithMTU(1500))
		if err != nil {
			t.Fatal(err)
		}
		network, err := NewNetwork(config)
		if err != nil {
			t.Fatal(err)
		}
		iface, err := StartInterfaceWithNetwork(network, WithInterfaceTSO(true))
		if err != nil {
			t.Fatal(err)
		}
		if iface.MTU() != 1500 {
			t.Errorf("MTU = %d, want 1500", iface.MTU())
		}
		if err := iface.Stop(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNetworkConfigurationNilOption(t *testing.T) {
	config, err := NewNetworkConfiguration(Mode(-1), nil)
	if config != nil || err == nil || !strings.Contains(err.Error(), "option is nil") {
		t.Fatalf("configuration = %v, error = %v", config, err)
	}
}

func TestWithMTUReuse(t *testing.T) {
	option := WithMTU(1500)
	for range 2 {
		var options networkConfigurationOptions
		if err := option(&options); err != nil {
			t.Fatal(err)
		}
		if err := WithMTU(9000)(&options); err != nil {
			t.Fatal(err)
		}
		if options.mtu != 9000 {
			t.Fatalf("MTU = %d, want 9000", options.mtu)
		}
	}
}
