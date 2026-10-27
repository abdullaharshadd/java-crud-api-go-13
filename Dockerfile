FROM golang:1.23-alpine

WORKDIR /app

# Allow the go command to resolve and record missing module checksums
# (go.sum is not committed), so builds don't fail with "missing go.sum entry".
ENV GOFLAGS=-mod=mod
ENV GOTOOLCHAIN=auto

COPY go.mod ./
COPY go.sum* ./
RUN apk add --no-cache git build-base && go mod download

COPY . .

RUN mkdir -p /app/bin && go mod tidy && go build -o /app/bin/server ./cmd/server

EXPOSE 8080

CMD ["sh", "-c", "/app/bin/server"]
