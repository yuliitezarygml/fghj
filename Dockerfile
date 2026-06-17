# Stage 1: Build the Go application
FROM golang:1.22-alpine AS builder

# Install certificates and git
RUN apk update && apk add --no-cache ca-certificates git

WORKDIR /app

# Copy dependency files and download
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application source code
COPY main.go ./
COPY internal/ ./internal/

# Build a statically linked binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o backend .

# Stage 2: Create the final lean image
FROM alpine:latest

# Install certificates for HTTPS requests (Steam API, Telegram, GitHub, Supabase)
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the built binary
COPY --from=builder /app/backend .

# Create the folder where manifests will be saved
RUN mkdir -p /app/manifests

# Expose a volume for the downloaded manifests and lua files
VOLUME ["/app/manifests"]

# Run the app in bot mode by default
ENTRYPOINT ["./backend"]
