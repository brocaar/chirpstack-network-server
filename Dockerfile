FROM golang:1.11-alpine AS development

ENV PROJECT_PATH=/go/src/github.com/brocaar/loraserver
ENV PATH=$PATH:$PROJECT_PATH/build
ENV CGO_ENABLED=0
ENV GO_EXTRA_BUILD_ARGS="-a -installsuffix cgo"

RUN apk add --no-cache ca-certificates tzdata make git bash protobuf

RUN mkdir -p $PROJECT_PATH
COPY . $PROJECT_PATH
WORKDIR $PROJECT_PATH

RUN make dev-requirements requirements
RUN make

FROM alpine:latest AS production

WORKDIR /root/
RUN apk --no-cache add ca-certificates tzdata
COPY --from=development /go/src/github.com/brocaar/loraserver/build/loraserver .
ENTRYPOINT ["./loraserver"]
