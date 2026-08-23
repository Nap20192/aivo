# Builds cmd/aivo-server plus the three SPAs (admin, menu, pos) so the
# compose happy path serves the real apps. cmd/aivo-seed stays a
# `go run` dev tool (see README).
FROM golang:1.26-alpine AS build
# go.mod pins a patch version newer than this base image ships; let Go
# fetch the exact toolchain it declares instead of pinning a base image
# tag we'd have to bump in lockstep with go.mod.
ENV GOTOOLCHAIN=auto
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
RUN CGO_ENABLED=0 go build -o /out/aivo-server ./cmd/aivo-server

FROM node:22-alpine AS frontendbuild
WORKDIR /frontend
# The SPAs import shared tokens from frontend/design-system — copy the whole
# frontend tree so relative imports resolve.
COPY frontend ./
RUN for app in admin menu pos; do \
      cd /frontend/$app && npm ci && npm run build; \
    done

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/aivo-server /usr/local/bin/aivo-server
# main.go serves the SPAs from relative paths ("frontend/<app>/dist"), so the
# dists live under /frontend given WORKDIR /.
COPY --from=frontendbuild /frontend/admin/dist /frontend/admin/dist
COPY --from=frontendbuild /frontend/menu/dist /frontend/menu/dist
COPY --from=frontendbuild /frontend/pos/dist /frontend/pos/dist
WORKDIR /
EXPOSE 8080
ENTRYPOINT ["aivo-server"]
