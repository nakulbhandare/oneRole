FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd/email_imap_watcher/ ./cmd/email_imap_watcher/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/email_imap_watcher ./cmd/email_imap_watcher

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/email_imap_watcher /app/email_imap_watcher
EXPOSE 3002
ENTRYPOINT ["/app/email_imap_watcher"]
