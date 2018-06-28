//go:generate protoc -I=. -I=$GOPATH/src --go_out=plugins=grpc:. nc.proto

package nc
