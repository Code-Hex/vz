.PHONY: fmt
fmt:
	@ls | grep -E '\.(h|m)$$' | xargs clang-format -i

.PHONY: test
test:
	go test -exec "go run $(PWD)/cmd/codesign" -count=1 ./...
