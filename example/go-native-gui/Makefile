FRAMEWORK ?= mygo

.PHONY: run build
run:
	go run -exec "go run ../../cmd/codesign/main.go" ./$(FRAMEWORK) $(ARGS)

build:
	go build -o virtualization ./$(FRAMEWORK)
