//go:generate protoc -I . --go_out=plugins=grpc:. profiles.proto ns.proto

package ns
