## Build
FROM golang:1.19-alpine AS build

ARG MODULE_NAME=github.com/ankorstore/mq-lease-service
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
FROM scratch
WORKDIR /
COPY --from=build /server /server

EXPOSE 8080
ENTRYPOINT [ "/server" ]
