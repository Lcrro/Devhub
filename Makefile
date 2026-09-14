BINARY := devhub

.PHONY: build run test fmt vet cross-build clean

build:
	go build -o $(BINARY) ./cmd/devhub

run:
	go run ./cmd/devhub

test:
	gofmt -w ./cmd ./internal
	go test ./...
	go vet ./...

fmt:
	gofmt -w ./cmd ./internal

vet:
	go vet ./...

cross-build:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/devhub-linux-amd64 ./cmd/devhub
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o dist/devhub-darwin-arm64 ./cmd/devhub
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o dist/devhub-windows-amd64.exe ./cmd/devhub

clean:
	rm -rf dist devhub
