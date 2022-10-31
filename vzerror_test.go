package vz_test

import (
	"testing"

	"github.com/Code-Hex/vz/v2"
)

func TestErrorCodeString(t *testing.T) {
	t.Run("fundamental error code", func(t *testing.T) {
		cases := []struct {
			code vz.ErrorCode
			want string
		}{
			{
				code: vz.ErrorInternal,
				want: "ErrorInternal",
			},
			{
				code: vz.ErrorInvalidVirtualMachineConfiguration,
				want: "ErrorInvalidVirtualMachineConfiguration",
			},
			{
				code: vz.ErrorInvalidVirtualMachineState,
				want: "ErrorInvalidVirtualMachineState",
			},
			{
				code: vz.ErrorInvalidVirtualMachineStateTransition,
				want: "ErrorInvalidVirtualMachineStateTransition",
			},
			{
				code: vz.ErrorInvalidDiskImage,
				want: "ErrorInvalidDiskImage",
			},
			{
				code: vz.ErrorVirtualMachineLimitExceeded,
				want: "ErrorVirtualMachineLimitExceeded",
			},
			{
				code: vz.ErrorNetworkError,
				want: "ErrorNetworkError",
			},

			{
				code: vz.ErrorOutOfDiskSpace,
				want: "ErrorOutOfDiskSpace",
			},
			{
				code: vz.ErrorOperationCancelled,
				want: "ErrorOperationCancelled",
			},
			{
				code: vz.ErrorNotSupported,
				want: "ErrorNotSupported",
			},
		}
		for _, tc := range cases {
			got := tc.code.String()
			if tc.want != got {
				t.Fatalf("want %q but got %q", tc.want, got)
			}
		}
	})

	t.Run("macOS installation", func(t *testing.T) {
		cases := []struct {
			code vz.ErrorCode
			want string
		}{
			{
				code: vz.ErrorRestoreImageCatalogLoadFailed,
				want: "ErrorRestoreImageCatalogLoadFailed",
			},
			{
				code: vz.ErrorInvalidRestoreImageCatalog,
				want: "ErrorInvalidRestoreImageCatalog",
			},
			{
				code: vz.ErrorNoSupportedRestoreImagesInCatalog,
				want: "ErrorNoSupportedRestoreImagesInCatalog",
			},
			{
				code: vz.ErrorRestoreImageLoadFailed,
				want: "ErrorRestoreImageLoadFailed",
			},
			{
				code: vz.ErrorInvalidRestoreImage,
				want: "ErrorInvalidRestoreImage",
			},
			{
				code: vz.ErrorInstallationRequiresUpdate,
				want: "ErrorInstallationRequiresUpdate",
			},
			{
				code: vz.ErrorInstallationFailed,
				want: "ErrorInstallationFailed",
			},
		}
		for _, tc := range cases {
			got := tc.code.String()
			if tc.want != got {
				t.Fatalf("want %q but got %q", tc.want, got)
			}
		}
	})
}
