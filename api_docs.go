package loraserver

import (
	"encoding/json"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

type rpcServiceDoc struct {
	Methods map[string]rpcMethodDoc `json:"methods"`
}

type rpcMethodDoc struct {
	ArgumentTypeName string `json:"argumentTypeName"`
	ReplyTypeName    string `json:"replyTypeName"`
	ArgumentJSON     string `json:"argumentJSON"`
	ReplyJSON        string `json:"replyJSON"`
	ArgumentPkgPath  string `json:"argumentPkgPath"`
	ReplyPkgPath     string `json:"replyPkgPath"`
}

func getRPCServicesDoc(rcvrs ...interface{}) (map[string]rpcServiceDoc, error) {
	services := make(map[string]rpcServiceDoc)

	for _, rcvr := range rcvrs {
		doc, err := getRPCServiceDoc(rcvr)
		if err != nil {
			return nil, err
		}
		services[getRPCServiceName(rcvr)] = doc
	}

	return services, nil
}

func getRPCServiceName(rcvr interface{}) string {
	return reflect.Indirect(reflect.ValueOf(rcvr)).Type().Name()
}

// major part of the code is borrowed from: https://golang.org/src/net/rpc/server.go

var typeOfError = reflect.TypeOf((*error)(nil)).Elem()

func getRPCServiceDoc(rcvr interface{}) (rpcServiceDoc, error) {
	typ := reflect.TypeOf(rcvr)
	docs := rpcServiceDoc{
		Methods: make(map[string]rpcMethodDoc),
	}

	for m := 0; m < typ.NumMethod(); m++ {
		method := typ.Method(m)
		mtype := method.Type
		mname := method.Name

		// method must be exported
		if method.PkgPath != "" {
			continue
		}

		// method needs three ins: receiver, args, *reply
		if mtype.NumIn() != 3 {
			continue
		}

		// First arg must be exported.
		argType := mtype.In(1)
		if !isExportedOrBuiltinType(argType) {
			continue
		}

		// Second arg must be a pointer.
		replyType := mtype.In(2)
		if replyType.Kind() != reflect.Ptr {
			continue
		}

		// Reply type must be exported.
		if !isExportedOrBuiltinType(replyType) {
			continue
		}

		// Method needs one out.
		if mtype.NumOut() != 1 {
			continue
		}

		// The return type of the method must be error.
		if returnType := mtype.Out(0); returnType != typeOfError {
			continue
		}

		// make sure we have the actual type instead of the pointers
		if argType.Kind() == reflect.Ptr {
			argType = argType.Elem()
		}
		replyType = replyType.Elem()

		obj := reflect.New(argType).Interface()
		b, err := json.MarshalIndent(struct {
			Method string        `json:"method"`
			Params []interface{} `json:"params"`
			ID     int           `json:"id"`
		}{
			Method: getRPCServiceName(rcvr) + "." + mname,
			Params: []interface{}{obj},
			ID:     1,
		}, "", "    ")
		if err != nil {
			return rpcServiceDoc{}, err
		}
		argumentJSON := string(b)

		// if the reply time is a slice, init the slice
		if replyType.Kind() == reflect.Slice {
			obj = []interface{}{reflect.New(replyType.Elem()).Interface()}
		} else {
			obj = reflect.New(replyType).Interface()
		}

		b, err = json.MarshalIndent(struct {
			Result interface{} `json:"result"`
			Error  *string     `json:"error"`
			ID     int         `json:"id"`
		}{obj, nil, 1}, "", "    ")
		if err != nil {
			return rpcServiceDoc{}, err
		}

		var replyTypeName, replyPkgPath string

		// if the reply type is a slice, take the actual type
		if replyType.Kind() == reflect.Slice {
			replyTypeName = replyType.Elem().Name()
			replyPkgPath = replyType.Elem().PkgPath()
		} else {
			replyTypeName = replyType.Name()
			replyPkgPath = replyType.PkgPath()
		}

		parts := strings.Split(argType.PkgPath(), "/vendor/")
		argumentPkgPath := parts[len(parts)-1]
		parts = strings.Split(replyPkgPath, "/vendor/")
		replyPkgPath = parts[len(parts)-1]

		docs.Methods[mname] = rpcMethodDoc{
			ArgumentJSON:     argumentJSON,
			ReplyJSON:        string(b),
			ArgumentTypeName: argType.Name(),
			ArgumentPkgPath:  argumentPkgPath,
			ReplyTypeName:    replyTypeName,
			ReplyPkgPath:     replyPkgPath,
		}

	}

	return docs, nil
}

// Is this an exported - upper case - name?
func isExported(name string) bool {
	rune, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(rune)
}

// Is this type exported or a builtin?
func isExportedOrBuiltinType(t reflect.Type) bool {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	// PkgPath will be non-empty even for an exported type,
	// so we need to check the type name as well.
	return isExported(t.Name()) || t.PkgPath() == ""
}
