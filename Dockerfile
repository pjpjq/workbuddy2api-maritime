# syntax=docker/dockerfile:1
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
ENV GOPROXY=https://proxy.golang.org,direct
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/wb2api ./cmd/server
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/wb-login ./cmd/login
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/wb-signin ./cmd/signin
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/wb-credit ./cmd/credit

FROM alpine:3.20
RUN apk add --no-cache curl wget ca-certificates tzdata bash python3 jq \
 && mkdir -p /app /data/auths /data/state
WORKDIR /app
COPY --from=build /out/wb2api /app/wb2api
COPY --from=build /out/wb-login /app/wb-login
COPY --from=build /out/wb-signin /app/wb-signin
COPY --from=build /out/wb-credit /app/wb-credit
COPY config.example.json /app/config.example.json
COPY login.sh /app/login.sh
COPY signin.sh /app/signin.sh
COPY credit.sh /app/credit.sh
COPY entrypoint.sh /app/entrypoint.sh

RUN chmod +x /app/wb2api /app/wb-login /app/wb-signin /app/wb-credit /app/login.sh /app/signin.sh /app/credit.sh /app/entrypoint.sh \
 && ln -sfn /app/wb-login /app/login \
 && ln -sfn /app/wb-signin /app/signin_bin \
 && ln -sfn /app/wb-credit /app/credit \
 && ln -sfn /data/auths /app/auths

ENV WB2A_AUTH_DIR=/data/auths \
    WB2A_STATE_FILE=/data/state.json

EXPOSE 18789
HEALTHCHECK --interval=15s --timeout=5s --start-period=5s \
  CMD wget -qO- http://127.0.0.1:${PORT:-18789}/health || exit 1

ENTRYPOINT ["/app/entrypoint.sh"]
