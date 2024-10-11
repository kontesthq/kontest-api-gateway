# Use the official Go image as a build stage
FROM golang:1.23.2-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum files to download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire project directory into the working directory
COPY . ./

# Build the Go application
RUN CGO_ENABLED=0 GOOS=linux go build -o /docker-gs-ping .

# Start a new scratch image for a smaller final image
FROM alpine:latest

# Copy the binary from the builder image
COPY --from=builder /docker-gs-ping /docker-gs-ping

# Set the command to run the binary
CMD ["/docker-gs-ping"]
