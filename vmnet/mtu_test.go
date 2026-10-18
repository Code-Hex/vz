package vmnet

import (
	"math"
	"strings"
	"testing"
)

func TestValidateMTU(t *testing.T) {
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
		err := validateMTU(test.mtu)
		if (err == nil) != test.valid {
			t.Errorf("validateMTU(%d) = %v, want valid %v", test.mtu, err, test.valid)
		}
	}
}

func TestSetMTUInvalidInputs(t *testing.T) {
	for _, config := range []*NetworkConfiguration{nil, {}} {
		for _, mtu := range []int{-1, 0, 1279, 65536, math.MaxInt} {
			if err := config.SetMTU(mtu); err == nil || !strings.Contains(err.Error(), "invalid vmnet MTU") {
				t.Errorf("SetMTU(%d) accepted an invalid MTU", mtu)
			}
		}
		if err := config.SetMTU(1500); err == nil {
			t.Error("SetMTU accepted an uninitialized configuration")
		}
	}
}

func TestSetMTUInterface(t *testing.T) {
	if Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	for _, mode := range []Mode{ModeHost, ModeShared} {
		config, err := NewNetworkConfiguration(mode)
		if err != nil {
			t.Fatal(err)
		}
		if err := config.SetMTU(1500); err != nil {
			t.Fatal(err)
		}
		network, err := NewNetwork(config)
		if err != nil {
			t.Fatal(err)
		}
		iface, err := StartInterfaceWithNetwork(network)
		if err != nil {
			t.Fatal(err)
		}
		if err := iface.Stop(); err != nil {
			t.Fatal(err)
		}
	}
}
