
# ---------- 构建阶段 ----------
FROM golang:1.27-bookworm AS builder

ARG VERSION=dev
WORKDIR /src

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}


# 先拷贝依赖清单，利用层缓存
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/team-router ./cmd/gateway

# ---------- 运行阶段 ----------
FROM alpine:3.23.7

ARG APK_MIRROR=mirrors.ustc.edu.cn

RUN sed -i "s/dl-cdn.alpinelinux.org/${APK_MIRROR}/g" /etc/apk/repositories \
    && adduser -D -u 10001 gateway \
    && apk add --no-cache ca-certificates tzdata
USER gateway

COPY --from=builder /out/team-router /usr/local/bin/team-router

EXPOSE 8080
ENTRYPOINT ["team-router"]
