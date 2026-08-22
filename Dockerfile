# Builds cmd/aivo-server plus the three SPAs (admin, menu, pos) so the
# compose happy path serves the real apps. cmd/aivo-seed stays a
# `go run` dev tool (see README).
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

FROM node:22-alpine AS webbuild
WORKDIR /web
# The SPAs import shared tokens from web/design-system — copy the whole
# web tree so relative imports resolve.
COPY web ./
RUN for app in admin menu pos; do \
      cd /web/$app && npm ci && npm run build; \
    done

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/aivo-server /usr/local/bin/aivo-server
# main.go serves the SPAs from relative paths ("web/<app>/dist"), so the
# dists live under /web given WORKDIR /.
COPY --from=webbuild /web/admin/dist /web/admin/dist
COPY --from=webbuild /web/menu/dist /web/menu/dist
COPY --from=webbuild /web/pos/dist /web/pos/dist
WORKDIR /
EXPOSE 8080
ENTRYPOINT ["aivo-server"]
