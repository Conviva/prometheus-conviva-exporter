FROM golang:alpine as base

RUN apk add --no-cache git bash \
  && rm -rf /var/cache/apk/*

WORKDIR /app

COPY ./go.mod ./go.sum ./main.go ./

ADD .git .git

RUN go mod download

# Build stage
FROM base as builder

RUN env CGO_ENABLED=0 go build -ldflags "-X main.Version=`git rev-parse --short HEAD`" -o /prometheus-conviva-exporter main.go

# Package stage
FROM alpine

COPY --from=builder /prometheus-conviva-exporter /bin/prometheus-conviva-exporter

RUN apk update \
    && apk --no-cache add ca-certificates

EXPOSE 8080

ENTRYPOINT  [ "/bin/prometheus-conviva-exporter" ]
