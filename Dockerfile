# === STAGE 1: The Builder ===
# We use the official Golang Alpine image as a build environment.
# 'golang:1.24.5-alpine' is a good choice as it is small and secure.
# Naming this stage 'builder' allows us to reference it later.
FROM golang:latest AS builder

# Set the working directory inside the container.
WORKDIR /app

# Copy the go.mod and go.sum files to leverage Docker's build cache.
# This step is cached and will only be re-run if these files change.
COPY go.mod go.sum ./

# Download all the Go modules needed for the project.
RUN go mod download

# Copy the entire source code into the container.
COPY . .

# Compile the Go application.
# -o /gtg-livemap-server: Specifies the output file name for the compiled binary.
# CGO_ENABLED=0: Disables CGO to create a statically linked binary without any C dependencies,
#                which is crucial for running it in a minimal 'alpine' image.
# -ldflags "-w -s": A production optimization that strips debug symbols and DWARF information,
#                   significantly reducing the binary's size.
RUN CGO_ENABLED=0 go build -ldflags "-w -s" -o /gtg-livemap-server .


# === STAGE 2: The Final Image ===
# We start from a fresh, minimal Alpine Linux image.
# This makes our final image very small and reduces the attack surface.
FROM alpine:latest

# It's good practice to install timezone data, which Go programs
# might need to handle timestamps correctly. `tzdata` is required for this.
RUN apk --no-cache add tzdata

# Copy only the compiled binary from the 'builder' stage.
# We don't need the Go compiler or the source code in our final image.
COPY --from=builder /gtg-livemap-server /gtg-livemap-server

# Copy the default configuration file. In production, this will typically
# be overridden by a volume mount, but it's good to have a default.
COPY config-example.yaml /config.yaml

# Copy the static frontend assets (HTML, JS, CSS, etc.) into the final image.
COPY static ./static

# Set the working directory for the running container.
WORKDIR /

# The EXPOSE instruction informs Docker that the container listens on the
# specified network ports at runtime. This is for documentation purposes.
EXPOSE 8080

# The ENTRYPOINT is the command that will be executed when the container starts.
# It runs our compiled application. It is configured to receive arguments via CMD.
ENTRYPOINT ["/gtg-livemap-server"]

# The CMD specifies the default arguments for the ENTRYPOINT.
# This will be overridden by the 'command' directive in the docker-compose.yaml file,
# allowing for flexible mode selection.
CMD ["-mode", "all"]