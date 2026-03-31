ARG TARGETOS
ARG TARGETARCH

FROM golang:1.26.1-alpine3.23 AS builder

WORKDIR /fluxsend

COPY . .

RUN go mod download

RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /fluxsend/fluxsend /fluxsend/cmd

EXPOSE 3000

FROM golang:1.26.1-alpine3.23

WORKDIR /fluxsend

COPY --from=builder /fluxsend/fluxsend /fluxsend/fluxsend
COPY --from=builder /fluxsend/internal/repo /fluxsend/internal/repo

CMD [ "/fluxsend/fluxsend" ]
