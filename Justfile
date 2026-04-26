# Run the example app
run:
    go run ./cmd/example/

# Build the example
build:
    go build ./cmd/example/

# Build the library
check:
    go build ./...
    go vet ./...

# Initialize beads database (if needed)
init:
    bd init
