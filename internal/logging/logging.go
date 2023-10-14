package logging

import (
	"context"
	"path"
	"time"

	"github.com/gofrs/uuid"
	grpc_logging "github.com/grpc-ecosystem/go-grpc-middleware/logging"
	grpc_logrus "github.com/grpc-ecosystem/go-grpc-middleware/logging/logrus"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/logrus/ctxlogrus"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

// ContextKey defines the context key type.
type ContextKey string

// ContextIDKey holds the key of the context ID.
const ContextIDKey ContextKey = "ctx_id"

type contextIDGetter interface {
	GetContextId() []byte
}

// UnaryServerCtxIDInterceptor adds the ContextIDKey to the context and sets
// it as a log field.
func UnaryServerCtxIDInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// generate unique id
	ctxID, err := uuid.NewV4()
	if err != nil {
		return nil, errors.Wrap(err, "new uuid error")
	}

	// set id to context and add as logrus field
	ctx = context.WithValue(ctx, ContextIDKey, ctxID)
	ctxlogrus.AddFields(ctx, logrus.Fields{
		"ctx_id": ctxID,
	})

	// set id as response header
	header := metadata.Pairs("ctx-id", ctxID.String())
	grpc.SendHeader(ctx, header)

	// execute the handler
	return handler(ctx, req)
}

func UnaryClientCtxIDInterceptor(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	// read reasponse meta-data (set by remote server)
	var header metadata.MD
	opts = append(opts, grpc.Header(&header))

	// set start time and invoke api methd
	startTime := time.Now()
	err := invoker(ctx, method, req, reply, cc, opts...)

	// get error code
	code := grpc_logging.DefaultErrorToCode(err)

	// get log-level for code
	level := grpc_logrus.DefaultCodeToLevel(code)

	// get log fields
	logFields := clientLoggerFields(ctx, method, reply, err, code, startTime, header)

	// log api call
	levelLogf(logrus.WithFields(logFields), level, "finished client unary call")

	return err
}

func clientLoggerFields(ctx context.Context, fullMethodString string, resp interface{}, err error, code codes.Code, start time.Time, header metadata.MD) logrus.Fields {
	service := path.Dir(fullMethodString)[1:]
	method := path.Base(fullMethodString)

	fields := logrus.Fields{
		"system":        "grpc",
		"span.kind":     "client",
		"grpc.service":  service,
		"grpc.method":   method,
		"grpc.duration": time.Since(start),
		"grpc.code":     code.String(),
		"ctx_id":        ctx.Value(ContextIDKey),
	}

	if err != nil {
		fields[logrus.ErrorKey] = err
	}

	// read context id from meta-data
	if values := header.Get("ctx-id"); len(values) != 0 {
		ctxID, err := uuid.FromString(values[0])
		if err == nil {
			fields["grpc.ctx_id"] = ctxID
		}
	}

	return fields
}

func levelLogf(entry *logrus.Entry, level logrus.Level, format string, args ...interface{}) {
	switch level {
	case logrus.DebugLevel:
		entry.Debugf(format, args...)
	case logrus.InfoLevel:
		entry.Infof(format, args...)
	case logrus.WarnLevel:
		entry.Warningf(format, args...)
	case logrus.ErrorLevel:
		entry.Errorf(format, args...)
	case logrus.FatalLevel:
		entry.Fatalf(format, args...)
	case logrus.PanicLevel:
		entry.Panicf(format, args...)
	}
}
