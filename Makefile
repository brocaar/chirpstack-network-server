.PHONY: build clean test package serve update-vendor api statics
PKGS := $(shell go list ./... | grep -v /vendor/ | grep -v loraserver/api | grep -v /migrations | grep -v /static)
VERSION := $(shell git describe --always)
GOOS ?= linux
GOARCH ?= amd64

build: statics
	@echo "Compiling source for $(GOOS) $(GOARCH)"
	@mkdir -p build
	@GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "-X main.version=$(VERSION)" -o build/loraserver$(BINEXT) cmd/loraserver/main.go

clean:
	@echo "Cleaning up workspace"
	@rm -rf build
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
	@cd docs/public/ && tar -pczf ../../dist/docs/loraserver.tar.gz .

package: clean build
	@echo "Creating package for $(GOOS) $(GOARCH)"
	@mkdir -p dist/tar/$(VERSION)
	@cp build/* dist/tar/$(VERSION)
	@cd dist/tar/$(VERSION) && tar -pczf ../loraserver_$(VERSION)_$(GOOS)_$(GOARCH).tar.gz .
	@rm -rf dist/tar/$(VERSION)

package-deb:
	@cd packaging && TARGET=deb ./package.sh

api:
	@echo "Generating API code from .proto files"
	@go generate api/as/as.go
	@go generate api/nc/nc.go
	@go generate api/ns/ns.go
	@go generate api/gw/gw.go

statics:
	@echo "Generating static files"
	@go generate cmd/loraserver/main.go

requirements:
	@go get -u github.com/golang/lint/golint
	@go get -u github.com/golang/protobuf/protoc-gen-go
	@go get -u github.com/golang/protobuf/proto
	@go get -u github.com/kardianos/govendor
	@go get -u github.com/elazarl/go-bindata-assetfs/...
	@go get -u github.com/jteeuwen/go-bindata/...

# shortcuts for development

serve: build
	@echo "Starting Lora Server"
	./build/loraserver

update-vendor:
	@echo "Updating vendored packages"
	@govendor update +external

run-compose-test:
	docker-compose run --rm loraserver make test
