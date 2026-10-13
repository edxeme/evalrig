BINARY := evalrig

.PHONY: build test vet fmt cover example integration clean

build:
	go build -o $(BINARY) ./cmd/evalrig

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

example: build
	./$(BINARY) run --suite examples/suite.yaml --agents golden

integration: build
	go test -race -tags integration ./internal/runtime -v

clean:
	rm -f $(BINARY) coverage.out
