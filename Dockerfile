# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS builder
WORKDIR /src

# Needed for HTTPS + timezone data (we will copy them into scratch)
RUN apk add --no-cache ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build args for multi-arch builds
ARG TARGETOS=linux
ARG TARGETARCH=amd64

# Build a static binary
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/asw-exporter .

# ---- minimal runtime ----
FROM scratch

# HTTPS root certs
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Timezone database (so LoadLocation("Europe/Berlin") works)
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
ENV TZ=Europe/Berlin

WORKDIR /app
COPY --from=builder /out/asw-exporter /app/asw-exporter

ENTRYPOINT ["/app/asw-exporter"]
