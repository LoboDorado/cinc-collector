FROM golang:1.27 AS builder
WORKDIR /src
COPY go.mod go.sum ./
COPY vendor/ vendor/
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 go build -mod=vendor -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}" \
    -o /out/cinc-collector ./cmd/cinc-collector

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/cinc-collector /cinc-collector
USER 65532:65532
EXPOSE 8891
HEALTHCHECK CMD ["/cinc-collector", "healthcheck"]
ENTRYPOINT ["/cinc-collector"]
CMD ["serve"]
