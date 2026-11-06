FROM mirror.gcr.io/library/golang:1.25-bookworm AS build
WORKDIR /src
COPY main.go .
RUN CGO_ENABLED=0 go build -trimpath -o /fixture main.go
FROM mirror.gcr.io/library/alpine:3.22
COPY --from=build /fixture /fixture
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/fixture"]
