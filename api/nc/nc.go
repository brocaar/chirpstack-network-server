//go:generate protoc -I=. -I=../.. --go_out=paths=source_relative,plugins=grpc:. nc.proto

package nc
