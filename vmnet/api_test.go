package vmnet_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Code-Hex/vz/v4/vmnet"
)

func TestVMNetPublicMethods(t *testing.T) {
	for _, tt := range []struct {
		typeOf reflect.Type
		want   []string
	}{
		{
			reflect.TypeFor[*vmnet.Network](),
			[]string{"CopySerialization", "IPv4Subnet", "IPv6Prefix"},
		},
		{
			reflect.TypeFor[*vmnet.NetworkConfiguration](),
			nil,
		},
	} {
		t.Run(tt.typeOf.Elem().Name(), func(t *testing.T) {
			var got []string
			for i := range tt.typeOf.NumMethod() {
				got = append(got, tt.typeOf.Method(i).Name)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("exported methods = %v, want %v", got, tt.want)
			}
		})
	}
}
