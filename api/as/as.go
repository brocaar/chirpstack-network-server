//go:generate protoc -I=. -I=$GOPATH/src --go_out=plugins=grpc:. as.proto

package as
