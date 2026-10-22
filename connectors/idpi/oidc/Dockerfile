# IdPi_OIDC connector module. Build from the repository root because the
# contract's generated code lives in the pm-core module until the SDK exists:
#   docker build -f connectors/idpi/oidc/Dockerfile -t pro-manage/idpi-oidc .
ARG GO_VERSION=1.26
FROM golang:${GO_VERSION}-alpine AS build
ARG VERSION=dev
ARG GIT_SHA=unknown
WORKDIR /src
COPY services/pm-core/go.mod services/pm-core/go.sum ./services/pm-core/
COPY connectors/idpi/oidc/go.mod connectors/idpi/oidc/go.sum ./connectors/idpi/oidc/
RUN cd connectors/idpi/oidc && go mod download
COPY services/pm-core/gen ./services/pm-core/gen
COPY connectors/idpi/oidc ./connectors/idpi/oidc
RUN cd connectors/idpi/oidc && CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags "-s -w -X github.com/reh3376/pro-manage/connectors/idpi/oidc/internal/runtime.Version=${VERSION} -X github.com/reh3376/pro-manage/connectors/idpi/oidc/internal/runtime.GitSHA=${GIT_SHA}" \
    -o /out/app ./cmd/idpi-oidc

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
COPY connectors/idpi/oidc/mapping.yaml /etc/idpi/mapping.yaml
USER nonroot:nonroot
ENTRYPOINT ["/app"]
