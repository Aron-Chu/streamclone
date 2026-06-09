FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG SERVICE
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/app ./cmd/${SERVICE} \
    && CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/healthcheck ./cmd/healthcheck

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/app /app
COPY --from=build /out/healthcheck /healthcheck
EXPOSE 8080
ENTRYPOINT ["/app"]
