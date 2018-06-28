//go:generate protoc -I=. -I=$GOPATH/src --go_out=plugins=grpc:. profiles.proto ns.proto

package ns
