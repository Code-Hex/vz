.PHONY: all
all: build codesign

.PHONY: codesign
codesign:
	codesign --entitlements vz.entitlements -s - ./virtualization

.PHONY: build
build:
	go build -o virtualization .
