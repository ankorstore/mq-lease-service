## Build
FROM golang:1.19-alpine AS build

ARG MODULE_NAME=github.com/ankorstore/gh-action-mq-lease-service
ARG BIN_NAME=server

ARG SHA
ARG TAG_NAME
ARG BUILD_DATE


ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
RUN go mod tidy

COPY . .

RUN go build \
    -ldflags '-X "$MODULE_NAME/internal/version.name=$BIN_NAME" -X "$MODULE_NAME/internal/version.commit=$SHA" -X "$MODULE_NAME/internal/version.date=$BUILD_DATE" -X "$MODULE_NAME/internal/version.tag=$TAG_NAME"' \
    -o /server \
    ./cmd/server/main.go

## Deploy
FROM busybox:1.36.0-uclibc as busybox
FROM gcr.io/distroless/base-debian10

WORKDIR /
COPY --from=build /server /server
# we use busybox and copy sh binary here to be able to do envvar interpolation in the entrypoint command (for log-level)
# so that it will be easy to override with a k8s envvar if we want to have debug logs without having to rebuild/deploy the app
COPY --from=busybox /bin/sh /bin/sh

EXPOSE 8080

ENTRYPOINT /server -config=/config.yaml -log-json=true -log-debug=$LOG_DEBUG -port=8080
