//go:generate protoc -I. -I$GOPATH/src -I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis --go_out=Mgoogle/api/annotations.proto=github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis/google/api,plugins=grpc:. application.proto node.proto
//go:generate protoc -I/usr/local/include -I. -I$GOPATH/src -I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis --grpc-gateway_out=logtostderr=true:. application.proto node.proto

package api
