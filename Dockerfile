# The KubeChat bot: a static Go binary in a minimal, non-root image.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
# Cache mounts keep rebuilds fast (`make dev`).
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /kubechat ./cmd/kubechat

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /kubechat /kubechat
USER 65532:65532
ENTRYPOINT ["/kubechat"]
