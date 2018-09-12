//go:generate protoc -I=. -I=$GOPATH/src --go_out=plugins=grpc:. geo.proto

package geo
