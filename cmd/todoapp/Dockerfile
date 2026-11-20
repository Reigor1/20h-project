FROM golang:1.26.0-bookworm AS builder

WORKDIR /app

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        gcc \
        libc6-dev && \
    rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 go build \
    -o /app/cmd/todoapp/exe \
    /app/cmd/todoapp

FROM debian:bookworm-slim

WORKDIR /app

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates \
        tzdata \
        libstdc++6 && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/cmd/todoapp/exe /app/exe

CMD ["/app/exe"]