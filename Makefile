.PHONY: build clean test package serve update-vendor
PKGS := $(shell go list ./... | grep -v /vendor/)
VERSION := $(shell git describe --always)
GOOS ?= linux
GOARCH ?= amd64
BANDS ?= eu_863_870

build:
	@echo "Compiling source"
	@for band in $(BANDS); do \
		echo "Compiling $$band for $(GOOS) $(GOARCH)" && \
		mkdir -p build/$$band && \
		GOOS=$(GOOS) GOARCH=$(GOARCH) go build -tags $$band -ldflags "-X main.version=$(VERSION)" -o build/$$band/loraserver$(BINEXT) cmd/loraserver/main.go ; \
	done

clean:
	@echo "Cleaning up workspace"
	@rm -rf build
	@rm -rf dist/$(VERSION)

test:
	@echo "Running tests"
	@for pkg in $(PKGS) ; do \
		golint $$pkg ; \
	done
	@go vet $(PKGS)
	@go test -p 1 -cover -v $(PKGS) -tags eu_863_870

package: clean build
	@echo "Creating package"
	@for band in $(BANDS); do \
		echo "Packaging $$band for $(GOOS) $(GOARCH)" && \
		mkdir -p dist/$(VERSION)/$$band && \
		cp build/$$band/* dist/$(VERSION)/$$band && \
		cd dist/$(VERSION)/$$band && tar -pczf ../../loraserver_$(VERSION)_$${band}_$(GOOS)_$(GOARCH).tar.gz . && \
		cd ../../.. ; \
	done
	rm -rf dist/$(VERSION)

# shortcuts for development

serve: build
	@echo "Starting Lora Server for $(BANDS) ISM band (to change the band for development, set the BANDS environment variable to a single band of your choice)"
	@echo "See https://github.com/brocaar/lorawan/tree/master/band for all available bands"
	./build/$(BANDS)/loraserver

update-vendor:
	@echo "Updating vendored packages"
	@govendor update +external
