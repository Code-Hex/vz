.PHONY: fmt test test-asan
fmt:
	@ls | grep -E '\.(h|m)$$' | xargs clang-format -i

test:
	@go test -c . -o vz.test
	@codesign --entitlements ./example/linux/vz.entitlements -s - ./vz.test || true
	@./vz.test

test-asan:
	@CGO_CFLAGS=-fsanitize=address CGO_LDFLAGS=-fsanitize=address go test -c . -o vz.test
	@codesign --entitlements ./example/linux/vz.entitlements -s - ./vz.test || true
	@./vz.test
