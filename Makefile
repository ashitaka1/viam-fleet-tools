MODULE_BINARY := bin/fleet-tools
GO_SOURCES := $(shell find . -type f -name '*.go' -not -path './bin/*')

$(MODULE_BINARY): Makefile go.mod $(GO_SOURCES)
	GOOS=$(VIAM_BUILD_OS) GOARCH=$(VIAM_BUILD_ARCH) go build -o $(MODULE_BINARY) ./cmd/module

lint:
	gofmt -s -w .
	go vet ./...

update:
	go get go.viam.com/rdk@latest
	go mod tidy

test:
	go test ./...

module.tar.gz: meta.json $(MODULE_BINARY)
	strip $(MODULE_BINARY)
	tar czf $@ meta.json $(MODULE_BINARY)

module: test module.tar.gz

all: test module.tar.gz

setup:
	go mod tidy
