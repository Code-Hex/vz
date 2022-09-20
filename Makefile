.PHONY: fmt test
fmt:
	@ls | grep -E '\.(h|m)$$' | xargs clang-format -i

test:
	@go test -c . -o vz.test
	@codesign --entitlements ./example/linux/vz.entitlements -s - ./vz.test || true
	@./vz.test
