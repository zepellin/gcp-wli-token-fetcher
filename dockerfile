############################
# STEP 1 build executable binary
############################
FROM golang:alpine AS builder
# Install git.
# Git is required for fetching the dependencies.
RUN apk update && apk add --no-cache git ca-certificates
WORKDIR $GOPATH/go/k8s-client/
COPY . .
# Fetch dependencies.
# Using go get.
RUN go get -d -v
# Build the binary.
RUN go build -o /go/bin/gcp-wli-token-fetcher


############################
# STEP 2 build a small image
############################
FROM scratch
# Copy CA certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
# Copy our static executable.
COPY --from=builder /go/bin/gcp-wli-token-fetcher /go/bin/gcp-wli-token-fetcher
# Run the gke-metadata-server binary.
ENTRYPOINT ["/go/bin/gcp-wli-token-fetcher"]
