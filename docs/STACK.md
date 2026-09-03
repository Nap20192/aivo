# Tech stack (pinned versions)

Fixed at the point of the microservices split, so upgrades are deliberate, not accidental.

## Backend (`backend/go.mod`)

| Component | Version |
|---|---|
| Go | 1.27.0 |
| google.golang.org/grpc | v1.83.2 |
| google.golang.org/protobuf | v1.36.12 |
| github.com/jackc/pgx/v5 | v5.10.0 |
| buf (codegen/lint, `backend/proto/buf.yaml`) | v2 config |

## Frontend (`frontend/{admin,pos,menu}/package.json`)

| Component | Version |
|---|---|
| React | ^18.3.1 |
| TypeScript | ^5.6.3 |
| Vite | ^5.4.11 |

## Infra (`deploy/docker-compose.yml`, `deploy/Dockerfile*`)

| Component | Version |
|---|---|
| Postgres | 16 |
| MinIO | `minio/minio:latest` |
| Go build image | `golang:1.27-alpine` (`GOTOOLCHAIN=auto`) |
| Node build image | `node:22-alpine` |
| Runtime base | `alpine:3.20` |

`minio/minio:latest`/`minio/mc:latest` are unpinned — dev-only convenience, not used in any release path yet.
