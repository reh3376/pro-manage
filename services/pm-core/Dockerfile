# Builds any binary under cmd/ (pm-core, pm-workers, stub-mdemg).
#   docker build --build-arg CMD=pm-core -t pro-manage/pm-core services/pm-core
ARG GO_VERSION=1.26
FROM golang:${GO_VERSION}-alpine AS build
ARG CMD=pm-core
ARG VERSION=dev
ARG GIT_SHA=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags "-s -w -X github.com/reh3376/pro-manage/services/pm-core/internal/config.Version=${VERSION} -X github.com/reh3376/pro-manage/services/pm-core/internal/config.GitSHA=${GIT_SHA}" \
    -o /out/app ./cmd/${CMD}

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]
