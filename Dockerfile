# Builds cmd/aivo-server. cmd/aivo-seed stays a `go run` dev tool (see
# README) — it's a one-off script, not a long-running service worth its
# own image.
FROM golang:1.26-alpine AS build
# go.mod pins a patch version newer than this base image ships; let Go
# fetch the exact toolchain it declares instead of pinning a base image
# tag we'd have to bump in lockstep with go.mod.
ENV GOTOOLCHAIN=auto
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/aivo-server ./cmd/aivo-server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/aivo-server /usr/local/bin/aivo-server
# main.go serves the SPAs from relative paths ("web/admin/dist",
# "web/pos/dist", "web/menu[-app]"), so the whole web/ tree lives at
# /web given WORKDIR /. Missing dist dirs answer 503 until built.
COPY web /web
WORKDIR /
EXPOSE 8080
ENTRYPOINT ["aivo-server"]
