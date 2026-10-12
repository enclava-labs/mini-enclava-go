FROM golang:1.26.4-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/enclava ./cmd/enclava

FROM alpine:3.22

RUN addgroup -S enclava && adduser -S -G enclava enclava
WORKDIR /app
COPY --from=build /out/enclava /usr/local/bin/enclava
RUN mkdir -p /data && chown -R enclava:enclava /data

USER enclava
ENV APP_HOST=0.0.0.0
ENV APP_PORT=8080
ENV DATABASE_URL=/data/enclava.db
EXPOSE 8080

ENTRYPOINT ["enclava"]
