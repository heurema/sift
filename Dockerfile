FROM golang:1.25.5-alpine AS builder
WORKDIR /src

RUN apk add --no-cache ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY docs/contracts/source-registry.seed.json ./docs/contracts/source-registry.seed.json

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/siftd ./cmd/siftd

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
	&& addgroup -S app \
	&& adduser -S app -G app \
	&& install -d -o app -g app /app /app/docs/contracts /data/output

WORKDIR /app

COPY --from=builder /out/siftd /app/siftd
COPY --from=builder /src/docs/contracts/source-registry.seed.json /app/docs/contracts/source-registry.seed.json

ENV SIFTD_OUTPUT_DIR=/data/output

USER app

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=20s --retries=3 CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1

ENTRYPOINT ["/app/siftd"]
