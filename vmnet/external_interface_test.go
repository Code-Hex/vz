package vmnet

import (
	"strings"
	"testing"
)

func TestWithExternalInterface(t *testing.T) {
	option := WithExternalInterface("en0")
	var first, second networkConfigurationOptions
	for _, options := range []*networkConfigurationOptions{&first, &second} {
		if err := option(options); err != nil {
			t.Fatal(err)
		}
	}
	if err := WithExternalInterface("en1")(&first); err != nil {
		t.Fatal(err)
	}
	if first.externalInterface != "en1" || second.externalInterface != "en0" {
		t.Fatalf("interfaces = %q, %q, want en1, en0", first.externalInterface, second.externalInterface)
	}
}

func TestNetworkConfigurationInvalidExternalInterface(t *testing.T) {
	for _, name := range []string{"", "\x00en0", "en0\x00", "en0\x00en1"} {
		config, err := NewNetworkConfiguration(Mode(-1), WithExternalInterface(name), WithExternalInterface("en0"))
		if config != nil || err == nil || !strings.Contains(err.Error(), "invalid vmnet external interface") {
			t.Errorf("WithExternalInterface(%q): configuration = %v, error = %v", name, config, err)
		}
	}
}

func TestNetworkConfigurationExternalInterfaceHostMode(t *testing.T) {
	config, err := NewNetworkConfiguration(ModeHost, WithExternalInterface("en0"))
	if config != nil || err == nil || !strings.Contains(err.Error(), "external interface requires ModeShared") {
		t.Fatalf("configuration = %v, error = %v", config, err)
	}
}

func TestExternalInterfaceConfiguration(t *testing.T) {
	if Available(26) {
		t.Skip("vmnet networks require macOS 26 and its SDK")
	}
	names, err := BridgedInterfaces()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Skip("no physical network interface available")
	}
	config, err := NewNetworkConfiguration(ModeShared, WithExternalInterface(names[0]))
	if err != nil || config == nil {
		t.Fatalf("NewNetworkConfiguration = %v, %v", config, err)
	}
}
