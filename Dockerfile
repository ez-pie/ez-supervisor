##
# ---------- building ----------
##
FROM golang:alpine  AS builder

WORKDIR /build

ADD . ./

ENV GO111MODULE=on \
GOPROXY=https://goproxy.cn

RUN go mod download

RUN go build -o gin_docker .

##
# ---------- run ----------
##
FROM alpine:latest

WORKDIR /build/

COPY --from=builder /build .

EXPOSE 8080

ENTRYPOINT  ["./gin_docker"]
