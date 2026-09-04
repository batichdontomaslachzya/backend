FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY internal/ ./internal/
COPY processor/ ./processor/
RUN CGO_ENABLED=0 go build -trimpath -o /processor ./processor && mkdir -p /data/input /data/result

FROM scratch
COPY --from=build /processor /processor
COPY --from=build --chown=65532:65532 /data /data
USER 65532:65532
ENV DATA_DIR=/data
ENTRYPOINT ["/processor"]
