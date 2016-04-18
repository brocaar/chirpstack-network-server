PKGS := $(shell go list ./... | grep -v /vendor/)

lint:
	@echo "Running linters"
	@for pkg in $(PKGS) ; do \
		golint $$pkg ; \
	done
	@go vet $(PKGS)

test: lint
	@echo "Running tests"
	@go test -cover -v 
	@for band in eu_863_870 us_902_928 ; do \
		echo "Testing $$band band" && \
		go test -cover -v -tags $$band ./band ; \
	done

