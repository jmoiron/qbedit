.PHONY: build test fmt run

# DEFAULT: build the project
build:
	go build

# Run tests across all packages
test:
	go test ./...

install:
	go test ./... && go install

# Format Go code using goimports if available, else gofmt.
# Excludes hidden directories (e.g., .git, .gopath, .cache).
fmt:
	@find . -type f -name '*.go' -not -path './.*' -exec goimports -w {} +;

# Dev run (restarts automatically when changing files)
run:
	@reflex -r '\.(go|css|js|gohtml)$$' -v -s -- sh -c 'make && ./qbedit -v ./ftbquests'
