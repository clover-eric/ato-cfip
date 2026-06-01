FROM golang:1.22-alpine AS build

WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cfst-daemon ./cmd/cfst-daemon

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/cfst-daemon /app/cfst-daemon
COPY config.example.yaml /app/config.yaml
COPY ip.txt /app/ip.txt
COPY ipv6.txt /app/ipv6.txt
VOLUME ["/app/data"]
EXPOSE 8080
ENTRYPOINT ["/app/cfst-daemon", "-config", "/app/config.yaml"]
