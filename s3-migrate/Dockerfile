# ---- Build stage ----
FROM --platform=$BUILDPLATFORM golang:1.26.1-alpine3.23 AS builder

WORKDIR /s3-migrate

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH

RUN GOOS=$TARGETOS GOARCH=$TARGETARCH CGO_ENABLED=0 go build -o /s3-migrate/s3-migrate .

# ---- Final stage ----
FROM alpine:3.23

WORKDIR /s3-migrate

COPY --from=builder /s3-migrate/s3-migrate .

CMD ["./s3-migrate"]