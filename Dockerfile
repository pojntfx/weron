# Build container
FROM golang:bookworm AS build

# Setup environment
RUN mkdir -p /data
WORKDIR /data

# Build the release
COPY . .
RUN make build/weron

# Extract the release
RUN mkdir -p /out
RUN cp out/weron /out/weron

# Release container
FROM debian:bookworm

# Add certificates
RUN apt update
RUN apt install -y ca-certificates

# Add the release
COPY --from=build /out/weron /usr/local/bin/weron

CMD /usr/local/bin/weron
