# --- Stage 1: Build ---
# Use an official Go image as the builder.
FROM golang:1.21-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum files to download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the application, statically linking it and disabling CGO.
# This makes it suitable for a minimal 'distroless' image.
RUN CGO_ENABLED=0 go build -o /worker .

# --- Stage 2: Final Image ---
# Use a minimal 'distroless' image for the final container.
# It contains only the application and its runtime dependencies.
# This improves security and reduces the image size.
FROM gcr.io/distroless/static-debian11

# Copy the built binary from the 'builder' stage
COPY --from=builder /worker /worker

# Expose the port our metrics server will run on
EXPOSE 8080

# Set the entrypoint for the container
ENTRYPOINT ["/worker"]

