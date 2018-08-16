//go:generate protoc -I=. -I=$GOPATH/src --go_out=plugins=grpc:$GOPATH/src gw.proto

package gw
