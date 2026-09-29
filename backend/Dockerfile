# Orchestrator API image. Build from the backend/ directory:
#   docker build -t orchestrator-backend:<tag> backend
# The Kubernetes adapter shells out to kubectl, so the runtime image carries a
# checksum-verified kubectl matching the supported cluster minor version.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/orchestrator ./cmd/orchestrator

FROM alpine:3.22
ARG KUBECTL_VERSION=v1.36.1
ARG TARGETARCH=amd64
RUN apk add --no-cache ca-certificates \
 && wget -q -O /usr/local/bin/kubectl "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" \
 && wget -q -O /tmp/kubectl.sha256 "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl.sha256" \
 && echo "$(cat /tmp/kubectl.sha256)  /usr/local/bin/kubectl" | sha256sum -c - \
 && chmod 0755 /usr/local/bin/kubectl \
 && rm /tmp/kubectl.sha256
COPY --from=build /out/orchestrator /usr/local/bin/orchestrator
COPY deploy/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
ENV HOME=/tmp
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
