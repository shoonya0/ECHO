# Build stage
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# Runtime stage
FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -H app
USER app
COPY --from=build /out/server /server
# Configuration comes from environment variables (see .env.example).
# Logs go to stdout unless LOG_FILE is set.
EXPOSE 8080
ENTRYPOINT ["/server"]
