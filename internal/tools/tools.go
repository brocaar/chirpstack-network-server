// +build tools

package tools

import (
	_ "github.com/elazarl/go-bindata-assetfs"
	_ "github.com/golang/protobuf/protoc-gen-go"
	_ "github.com/goreleaser/goreleaser"
	_ "github.com/goreleaser/nfpm"
	_ "github.com/jteeuwen/go-bindata/go-bindata"
	_ "golang.org/x/lint/golint"
	_ "golang.org/x/tools/cmd/stringer"
)
