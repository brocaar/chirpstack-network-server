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
	@for pkg in $(PKGS) ; do \
		golint $$pkg ; \
	done
	@go vet $(PKGS)
	@go test -p 1 -v $(PKGS)

documentation:
	@echo "Building documentation"
	@mkdir -p dist/docs
	@cd docs && hugo
	@cd docs/public/ && tar -pczf ../../dist/loraserver-documentation.tar.gz .

dist:
	@goreleaser

build-snapshot:
	@goreleaser --snapshot

package-deb:
	@cd packaging && TARGET=deb ./package.sh

api:
	@echo "Generating API code from .proto files"
	go generate api/as/as.go
	go generate api/nc/nc.go
	go generate api/ns/ns.go
	go generate internal/storage/device_session.go

statics:
	@echo "Generating static files"
	@go generate cmd/loraserver/main.go

requirements:
	@go get -u github.com/kisielk/errcheck
	@go get -u github.com/golang/lint/golint
	@go get -u github.com/kardianos/govendor
	@go get -u github.com/smartystreets/goconvey
	@go get -u golang.org/x/tools/cmd/stringer
	@go get -u github.com/golang/protobuf/proto
	@go get -u github.com/golang/protobuf/protoc-gen-go
	@go get -u github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger
	@go get -u github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway
	@go get -u github.com/elazarl/go-bindata-assetfs/...
	@go get -u github.com/jteeuwen/go-bindata/...
	@go get -u github.com/golang/dep/cmd/dep
	@go get -u github.com/goreleaser/goreleaser
	@dep ensure -v

# shortcuts for development

serve: build
	@echo "Starting Lora Server"
	./build/loraserver

run-compose-test:
	docker-compose run --rm loraserver make test
