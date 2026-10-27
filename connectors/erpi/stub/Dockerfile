# ERPi_STUB connector module (development and CI ERP backend). Build from the repository root:
#   docker build -f connectors/erpi/stub/Dockerfile -t pro-manage/erpi-stub .
ARG GO_VERSION=1.26
FROM golang:${GO_VERSION}-alpine AS build
ARG VERSION=dev
WORKDIR /src
COPY services/pm-core/go.mod services/pm-core/go.sum ./services/pm-core/
COPY connectors/erpi/stub/go.mod connectors/erpi/stub/go.sum ./connectors/erpi/stub/
RUN cd connectors/erpi/stub && go mod download
COPY services/pm-core/gen ./services/pm-core/gen
COPY connectors/erpi/stub ./connectors/erpi/stub
RUN cd connectors/erpi/stub && CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags "-s -w -X main.Version=${VERSION}" -o /out/app ./cmd/erpi-stub

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]
