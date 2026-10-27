FROM golang:1.27-alpine

WORKDIR /app

COPY . .

RUN apk add --no-cache git build-base && mkdir -p /app/bin && cd /app && go mod tidy && go build -o /app/bin/server ./cmd/server

EXPOSE 8080

CMD ["sh", "-c", "/app/bin/server"]
