# The acceptance workloads are statically linked Go binaries, so a scratch image
# keeps the build free of any registry pull.
FROM scratch
ARG BINARY=app
COPY ${BINARY} /app
ENTRYPOINT ["/app"]
