# ---- Build stage ----
FROM --platform=$BUILDPLATFORM golang:1.26.4-alpine3.23 AS builder

WORKDIR /fluxsend

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH

RUN GOOS=$TARGETOS GOARCH=$TARGETARCH CGO_ENABLED=0 go build -o /fluxsend/fluxsend ./cmd

# ---- Final stage ----
FROM alpine:3.23

WORKDIR /fluxsend

COPY --from=builder /fluxsend/fluxsend .
COPY --from=builder /fluxsend/internal/repo/migrations ./internal/repo/migrations

EXPOSE 3000

CMD ["./fluxsend"]