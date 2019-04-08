//go:generate protoc -I=. -I=../.. --go_out=paths=source_relative,plugins=grpc:. as.proto

package as
