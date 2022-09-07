.PHONY: all
all: build codesign

.PHONY: codesign
codesign:
	codesign --entitlements vz.entitlements -s - ./virtualization

.PHONY: build
build:
	go build -o virtualization .

.PHONY: cross-build virtualization-amd64 virtualization-arm64
cross-build: virtualization-amd64 virtualization-arm64

virtualization-amd64 virtualization-arm64: virtualization-%:
	CGO_ENABLED=1 GOOS=darwin GOARCH=$* go build -o $@ .
