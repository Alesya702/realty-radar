# Builds the single Realty Radar binary. Build context: repository root.

FROM golang:1.27-alpine AS build
WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY src ./src
COPY migrations ./migrations

ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/realtyradar ./src/cmd/realtyradar

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/realtyradar /realtyradar
EXPOSE 8080
ENTRYPOINT ["/realtyradar"]
CMD ["serve"]
