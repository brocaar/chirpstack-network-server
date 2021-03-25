// +build tools

package tools

import (
	_ "github.com/golang/protobuf/protoc-gen-go"
	_ "github.com/goreleaser/goreleaser"
	_ "github.com/goreleaser/nfpm"
	_ "golang.org/x/lint/golint"
	_ "golang.org/x/tools/cmd/stringer"
)
