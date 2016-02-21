PKGS := $(shell go list ./... | grep -v /vendor/)
VERSION := $(shell git describe --always)

build:
	@echo "Compiling source"
	@mkdir -p bin
	@GOBIN="$(CURDIR)/bin" go install -ldflags "-X main.version=$(VERSION)" $(PKGS)

clean:
	@echo "Cleaning up workspace"
	@rm -rf bin

test:
	@echo "Running tests"
	@for pkg in $(PKGS) ; do \
		golint $$pkg ; \
	done
	@go vet $(PKGS)
	@go test -cover $(PKGS)

# shortcuts for development

serve: build
	./bin/loraserver

update-vendor:
	@echo "Updating vendored packages"
	@govendor update +external
