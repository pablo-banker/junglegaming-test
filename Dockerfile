# syntax=docker/dockerfile:1

# Go version must match go.mod.
FROM golang:1.27.1-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 go build -mod=mod -trimpath -ldflags="-s -w" -o /out/server ./cmd/server


FROM gcr.io/distroless/static-debian13:nonroot

COPY --from=build /out/server /server

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/server"]
