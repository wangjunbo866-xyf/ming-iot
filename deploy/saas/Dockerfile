# 本地构建 ming-iot core (单体 saassvr) 的最小镜像。
# 不依赖任何私有 registry；基础镜像来自 Docker Hub 官方。
# spec 非目标明确不迁入依赖私有镜像/远程前端下载的旧 Dockerfile，本文件是重新设计的容器化入口。

# ---------- builder ----------
FROM golang:1.24-alpine AS go-builder

# git: go mod 需要；ca-certificates: 拉取私有 gitee module 时校验证书
RUN apk add --no-cache git ca-certificates

WORKDIR /src

# 先拷 go.mod/go.sum 利用层缓存
COPY backend/saas/go.mod backend/saas/go.sum ./
# share 是 gitee 私有依赖，GONOSUMCHECK 允许跳过 sumdb 校验
ENV GONOSUMCHECK=*
ENV GONOSUMDB=*
ENV GOPROXY=https://goproxy.cn,direct
RUN go mod download

# 拷入完整源码并构建单体 saassvr (apisvr 入口)
COPY backend/saas/ ./
RUN go build -tags no_k8s -trimpath -o /out/saassvr ./service/apisvr

# ---------- runtime ----------
FROM alpine:3.20

ENV TZ=Asia/Shanghai
RUN apk add --no-cache tzdata ca-certificates && \
    cp /usr/share/zoneinfo/$TZ /etc/localtime && \
    echo $TZ > /etc/timezone

WORKDIR /unitedrhino
COPY --from=go-builder /out/saassvr ./saassvr

# etc/ 由 docker-compose 以 volume 挂载（deploy/saas/etc），便于不重建镜像即改配置
# oss 本地存储目录
RUN mkdir -p ./oss/ithings-public ./oss/ithings-private ./oss/ithings-temporary

EXPOSE 7777 9180
ENTRYPOINT ["./saassvr"]
