# wuxing kernel daemon — multi-stage build, distroless final image.

# --- build stage ---
FROM golang:1.23 AS build

WORKDIR /src

# Cache deps first.
COPY go.mod go.sum* ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/wuxing ./cmd/wuxing

# --- runtime stage ---
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/wuxing /usr/local/bin/wuxing
COPY manifest/ /etc/wuxing/manifest/

USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/wuxing"]
CMD ["--manifest", "/etc/wuxing/manifest/boot.yml"]
