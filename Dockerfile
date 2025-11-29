FROM golang:1.24 AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/haproxy-k8s-sync-controller ./cmd/haproxy-k8s-sync-controller

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /out/haproxy-k8s-sync-controller /haproxy-k8s-sync-controller

USER nonroot:nonroot
ENTRYPOINT ["/haproxy-k8s-sync-controller"]
