.PHONY: build clean test package serve update-vendor api statics
PKGS := $(shell go list ./... | grep -v /vendor/ | grep -v loraserver/api | grep -v /migrations | grep -v /static)
VERSION := $(shell git describe --always |sed -e "s/^v//")

build: statics
	@echo "Compiling source"
	@mkdir -p build
	go build $(GO_EXTRA_BUILD_ARGS) -ldflags "-s -w -X main.version=$(VERSION)" -o build/loraserver cmd/loraserver/main.go

clean:
	@echo "Cleaning up workspace"
	@rm -rf build
	@rm -rf dist
	@rm -rf docs/public

test: statics
	@echo "Running tests"
	@rm -f coverage.out
	@for pkg in $(PKGS) ; do \
		golint $$pkg ; \
	done
	@go vet $(PKGS)
	@go test -p 1 -v -cover $(PKGS) -coverprofile coverage.out

dist: statics
	goreleaser
	mkdir -p dist/upload/tar
	mkdir -p dist/upload/deb
	mv dist/*.tar.gz dist/upload/tar
	mv dist/*.deb dist/upload/deb

snapshot:
	@goreleaser --snapshot

api:
	@echo "Generating API code from .proto files"
	go generate api/gw/gw.go
	go generate api/as/as.go
	go generate api/nc/nc.go
	go generate api/ns/ns.go
	go generate api/geo/geo.go
	go generate api/common/common.go
	go generate internal/storage/device_session.go

statics:
	@echo "Generating static files"
	@go generate internal/migrations/migrations.go

dev-requirements:
	go install golang.org/x/lint/golint
	go install golang.org/x/tools/cmd/stringer
	go install github.com/golang/protobuf/protoc-gen-go
	go install github.com/elazarl/go-bindata-assetfs/go-bindata-assetfs
	go install github.com/jteeuwen/go-bindata/go-bindata
	go install github.com/goreleaser/goreleaser
	go install github.com/goreleaser/nfpm

# shortcuts for development

serve: build
	@echo "Starting Lora Server"
	./build/loraserver

run-compose-test:
	docker-compose run --rm loraserver make test
