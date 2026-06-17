FROM --platform=$BUILDPLATFORM golang:1.23-bookworm AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN target_os="${TARGETOS:-linux}" && \
    target_arch="${TARGETARCH:-$(go env GOARCH)}" && \
    CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -o /out/worker ./cmd/worker

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/api /api
COPY --from=build /out/worker /worker
CMD ["/api"]
