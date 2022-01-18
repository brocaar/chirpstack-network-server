.PHONY: build clean test package serve update-vendor api
VERSION := $(shell git describe --always |sed -e "s/^v//")
API_VERSION := $(shell go list -m -f '{{ .Version }}' github.com/brocaar/chirpstack-api/go/v3 | awk '{n=split($$0, a, "-"); print a[n]}')

build:
	@echo "Compiling source"
	@mkdir -p build
	go build $(GO_EXTRA_BUILD_ARGS) -ldflags "-s -w -X main.version=$(VERSION)" -o build/chirpstack-network-server cmd/chirpstack-network-server/main.go

clean:
	@echo "Cleaning up workspace"
	@rm -rf build
	@rm -rf dist

test:
	@echo "Running tests"
	@rm -f coverage.out
	@golint ./...
	@go vet ./...
	@go test -p 1 -v -cover ./... -coverprofile coverage.out

dist:
	goreleaser
	mkdir -p dist/upload/tar
	mkdir -p dist/upload/deb
	mkdir -p dist/upload/rpm
	mv dist/*.tar.gz dist/upload/tar
	mv dist/*.deb dist/upload/deb
	mv dist/*.rpm dist/upload/rpm

snapshot:
	@goreleaser --snapshot

api:
	@echo "Fetching Protobuf API files"
	@rm -rf /tmp/chirpstack-api
	@git clone https://github.com/brocaar/chirpstack-api.git /tmp/chirpstack-api
	@git --git-dir=/tmp/chirpstack-api/.git --work-tree=/tmp/chirpstack-api checkout go/$(API_VERSION)

	@echo "Generating API code from .proto files"
	go generate internal/storage/device_session.go
	go generate internal/storage/downlink_frame.go

dev-requirements:
	go install golang.org/x/lint/golint
	go install golang.org/x/tools/cmd/stringer
	go install github.com/golang/protobuf/protoc-gen-go
	go install github.com/goreleaser/goreleaser
	go install github.com/goreleaser/nfpm

# shortcuts for development

serve: build
	@echo "Starting ChirpStack Network Server"
	./build/chirpstack-network-server

run-compose-test:
	docker-compose run --rm chirpstack-network-server make test
