//go:generate protoc -I=. -I=../.. --go_out=paths=source_relative,plugins=grpc:. geo.proto

package geo
