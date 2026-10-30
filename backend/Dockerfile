# Orchestrator API image. Build from the backend/ directory:
#   docker build -t orchestrator-backend:<tag> backend
# The Kubernetes adapter shells out to kubectl, so the runtime image carries a
# checksum-verified kubectl matching the supported cluster minor version.
FROM mirror.gcr.io/library/golang:1.25-alpine AS build
WORKDIR /src

# Install CA certificates for Go module downloads (development only)
RUN echo "http://dl-cdn.alpinelinux.org/alpine/v3.22/main" > /etc/apk/repositories \
 && echo "http://dl-cdn.alpinelinux.org/alpine/v3.22/community" >> /etc/apk/repositories \
 && apk add --no-cache ca-certificates

# Disable SSL verification for Go (development only)
ENV GODEBUG=x509ignoreCN=0
ENV GIT_SSL_NO_VERIFY=true
ENV GOPROXY=http://proxy.golang.org,direct
ENV GOINSECURE=*

COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/orchestrator ./cmd/orchestrator

# Only image preparation needs Internet. Download every release provider and
# verify its archive against the embedded release's zh: hashes.
FROM build AS provider-mirror
ARG TARGETARCH=amd64
RUN go run ./cmd/prepare-provider-mirror -dir /out/provider-mirror -platform linux_${TARGETARCH}

FROM build AS offline-check-build
RUN CGO_ENABLED=0 go test -c -tags offlineprovider -o /out/offline-provider-check ./internal/adapters/terraform/runner

FROM mirror.gcr.io/library/alpine:3.22 AS runtime
ARG KUBECTL_VERSION=v1.36.1
ARG TARGETARCH=amd64

# Disable SSL verification for apk and wget (development only)
RUN echo "http://dl-cdn.alpinelinux.org/alpine/v3.22/main" > /etc/apk/repositories \
 && echo "http://dl-cdn.alpinelinux.org/alpine/v3.22/community" >> /etc/apk/repositories \
 && apk add --no-cache ca-certificates git \
 && wget --no-check-certificate -q -O /usr/local/bin/kubectl "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" \
 && wget --no-check-certificate -q -O /tmp/kubectl.sha256 "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl.sha256" \
 && echo "$(cat /tmp/kubectl.sha256)  /usr/local/bin/kubectl" | sha256sum -c - \
 && chmod 0755 /usr/local/bin/kubectl \
 && rm /tmp/kubectl.sha256
ARG SCORE_K8S_VERSION=0.15.0
RUN wget --no-check-certificate -q -O /tmp/score.tgz "https://github.com/score-spec/score-k8s/releases/download/${SCORE_K8S_VERSION}/score-k8s_${SCORE_K8S_VERSION}_linux_${TARGETARCH}.tar.gz" \
 && tar -xzf /tmp/score.tgz -C /usr/local/bin score-k8s && chmod 0755 /usr/local/bin/score-k8s && rm /tmp/score.tgz
COPY --from=hashicorp/terraform:1.9.8 /bin/terraform /usr/local/bin/terraform
COPY --from=build /out/orchestrator /usr/local/bin/orchestrator
COPY --from=provider-mirror /out/provider-mirror /usr/local/share/orchestrator/provider-mirror
COPY deploy/terraform-offline.tfrc /etc/orchestrator/terraform-offline.tfrc
COPY deploy/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
ENV HOME=/tmp
ENV ORCHESTRATOR_TERRAFORM_CLI_CONFIG_FILE=/etc/orchestrator/terraform-offline.tfrc
# Durable verified Git render bundle content (ADR-021); mount its own volume.
ENV ORCHESTRATOR_RENDER_BUNDLE_CACHE_DIR=/var/lib/orchestrator/render-bundle-cache
RUN mkdir -p /var/lib/orchestrator/delivery-git /var/lib/orchestrator/recovery /var/lib/orchestrator/tfroot /var/lib/orchestrator/plugin-cache /var/lib/orchestrator/module-cache /var/lib/orchestrator/render-bundle-cache && chown -R 65532:65532 /var/lib/orchestrator && chmod -R 0700 /var/lib/orchestrator
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]

# Run this target with --network none. It uses the production runner, real
# packaged modules, release locks and image providers, without plan/apply.
FROM runtime AS offline-provider-check
COPY --from=offline-check-build /out/offline-provider-check /usr/local/bin/offline-provider-check
ENTRYPOINT ["/usr/local/bin/offline-provider-check", "-test.run=^TestOfflinePackagedModules$", "-test.v"]

FROM runtime AS final
