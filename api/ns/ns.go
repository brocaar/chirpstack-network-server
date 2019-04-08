//go:generate protoc -I=. -I=../.. --go_out=paths=source_relative,plugins=grpc:. profiles.proto ns.proto

package ns
