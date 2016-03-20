PKGS := $(shell go list ./... | grep -v /vendor/)
VERSION := $(shell git describe --always)

build:
	@echo "Compiling source"
	@mkdir -p bin
	@GOBIN="$(CURDIR)/bin" go install -ldflags "-X main.version=$(VERSION)" $(PKGS)

clean:
	@echo "Cleaning up workspace"
	@rm -rf bin
	@rm -rf builds

test:
	@echo "Running tests"
	@for pkg in $(PKGS) ; do \
		golint $$pkg ; \
	done
	@go vet $(PKGS)
	@go test -p 1 -cover -v $(PKGS)

package: clean build
	@echo "Creating package"
	@mkdir -p builds/$(VERSION)
	@cp bin/* builds/$(VERSION)
	@cd builds/$(VERSION)/ && tar -pczf ../loraserver_$(VERSION)_linux_amd64.tar.gz .
	@rm -rf builds/$(VERSION)

# shortcuts for development

serve: build
	./bin/loraserver

update-vendor:
	@echo "Updating vendored packages"
	@govendor update +external
