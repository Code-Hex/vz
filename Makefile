PUIPUI_LINUX_VERSION := 0.0.1
KERNEL_ARCH := $(shell uname -m | sed -e s/arm64/aarch64/)
KERNEL_TAR := puipui_linux_v$(PUIPUI_LINUX_VERSION)_$(KERNEL_ARCH).tar.gz
KERNEL_DOWNLOAD_URL := https://github.com/Code-Hex/puipui-linux/releases/download/v$(PUIPUI_LINUX_VERSION)/$(KERNEL_TAR)

.PHONY: fmt
fmt:
	@ls | grep -E '\.(h|m)$$' | xargs clang-format -i

.PHONY: test
test:
	go test -exec "go run $(PWD)/cmd/codesign" -count=1 ./... -timeout 60s

.PHONY: download_kernel
download_kernel:
ifneq ("$(wildcard $(KERNEL_TAR))","")
	curl --output-dir testdata -LO $(KERNEL_DOWNLOAD_URL)
endif
	@tar xvf testdata/$(KERNEL_TAR) -C testdata
ifeq ($(shell uname -m), arm64)
	@gunzip -f testdata/Image.gz
else
	@mv testdata/bzImage testdata/Image
endif