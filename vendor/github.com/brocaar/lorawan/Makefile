PKGS := $(shell go list ./... | grep -v /vendor/)

lint:
	@echo "Running linters"
	@for pkg in $(PKGS) ; do \
		golint $$pkg ; \
	done
	@go vet $(PKGS)

test: lint
	@echo "Running tests"
	@go test -cover -v ./...
