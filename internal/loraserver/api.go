package loraserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/rpc"
	"net/rpc/jsonrpc"

	log "github.com/Sirupsen/logrus"
)

// JSONRPCHandler implements a http.Handler compatible JSON-RPC handler.
type JSONRPCHandler struct {
	server *rpc.Server
	docs   map[string]rpcServiceDoc
}

// NewJSONRPCHandler creates a new JSONRPCHandler.
func NewJSONRPCHandler(srvcs ...interface{}) (http.Handler, error) {
	s := rpc.NewServer()
	for _, srvc := range srvcs {
		if err := s.RegisterName(getRPCServiceName(srvc), srvc); err != nil {
			return nil, fmt.Errorf("register rpc service error: %s", err)
		}
	}
	docs, err := getRPCServicesDoc(srvcs...)
	if err != nil {
		return nil, err
	}
	return &JSONRPCHandler{s, docs}, nil
}

// ServeHTTP implements the http.Handler interface.
func (h *JSONRPCHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		enc := json.NewEncoder(w)
		if err := enc.Encode(h.docs); err != nil {
			log.Errorf("marshal rpc docs error: %s", err)
		}
		return
	}

	conn := struct {
		io.Writer
		io.ReadCloser
	}{w, r.Body}

	if err := h.server.ServeRequest(jsonrpc.NewServerCodec(conn)); err != nil {
		log.Errorf("rpc request error: %s", err)
	}
}
