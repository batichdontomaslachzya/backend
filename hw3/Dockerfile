FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY docs/ ./docs/
COPY internal/ ./internal/
COPY migrations/ ./migrations/
RUN CGO_ENABLED=0 go build -trimpath -o /server . && mkdir -p /data/input /data/result /runtime-tmp

FROM scratch
COPY --from=build /server /server
COPY --from=build --chown=65532:65532 /data /data
COPY --from=build --chown=65532:65532 /runtime-tmp /tmp
USER 65532:65532
EXPOSE 8000
ENV DATA_DIR=/data
ENTRYPOINT ["/server"]
