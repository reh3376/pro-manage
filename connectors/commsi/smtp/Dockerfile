# Commsi_SMTP connector module. Build from the repository root:
#   docker build -f connectors/commsi/smtp/Dockerfile -t pro-manage/commsi-smtp .
ARG GO_VERSION=1.26
FROM golang:${GO_VERSION}-alpine AS build
ARG VERSION=dev
WORKDIR /src
COPY services/pm-core/go.mod services/pm-core/go.sum ./services/pm-core/
COPY connectors/commsi/smtp/go.mod connectors/commsi/smtp/go.sum ./connectors/commsi/smtp/
RUN cd connectors/commsi/smtp && go mod download
COPY services/pm-core/gen ./services/pm-core/gen
COPY connectors/commsi/smtp ./connectors/commsi/smtp
RUN cd connectors/commsi/smtp && CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags "-s -w -X main.Version=${VERSION}" -o /out/app ./cmd/commsi-smtp

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]
