FROM golang:1.9-alpine3.6 AS development

ENV PROJECT_PATH=/go/src/github.com/brocaar/loraserver
ENV PATH=$PATH:$PROJECT_PATH/build

RUN apk add --no-cache ca-certificates tzdata make git bash protobuf

RUN mkdir -p $PROJECT_PATH
COPY . $PROJECT_PATH
WORKDIR $PROJECT_PATH

RUN make requirements
RUN make

FROM alpine:latest AS production

WORKDIR /root/
RUN apk --no-cache add ca-certificates tzdata
COPY --from=development /go/src/github.com/brocaar/loraserver/build/loraserver .
ENTRYPOINT ["./loraserver"]
