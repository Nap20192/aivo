# Builds cmd/menu-server. cmd/menu-seed stays a `go run` dev tool (see
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
RUN CGO_ENABLED=0 go build -o /out/menu-server ./cmd/menu-server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/menu-server /usr/local/bin/menu-server
# main.go serves the static frontend from the relative path "web/menu",
# so it has to live at /web/menu given WORKDIR /.
COPY web/menu /web/menu
WORKDIR /
EXPOSE 8080
ENTRYPOINT ["menu-server"]
