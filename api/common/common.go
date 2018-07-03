//go:generate protoc -I=. -I=$GOPATH/src --go_out=plugins=grpc:$GOPATH/src common.proto

package common
