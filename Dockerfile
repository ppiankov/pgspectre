FROM golang:1.25-alpine AS builder

ARG VERSION=dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /pgspectre ./cmd/pgspectre/

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /pgspectre /pgspectre

ENTRYPOINT ["/pgspectre"]
