.PHONY: build test fmt run

# DEFAULT: build the project
build:
	go build

# Run tests across all packages
test:
	go test ./...

# Format Go code using goimports if available, else gofmt.
# Excludes hidden directories (e.g., .git, .gopath, .cache).
fmt:
	@FILES=$$(find . -type d -name '.*' -prune -o -type f -name '*.go' -print); \
	if command -v goimports >/dev/null 2>&1; then \
		echo "Formatting with goimports"; \
		echo "$$FILES" | xargs -r goimports -w; \
	else \
		echo "goimports not found; using gofmt"; \
		echo "$$FILES" | xargs -r gofmt -w; \
	fi

# Dev run (restarts automatically when changing files)
run:
	@reflex -r '\.(go|css|js|gohtml)$$' -v -s -- sh -c 'make && ./qbedit ./ftbquests'
