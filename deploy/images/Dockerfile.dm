FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
ARG VCS_REF
ARG BINARY_SHA256
LABEL org.opencontainers.image.revision=$VCS_REF \
      io.ming-iot.binary.sha256=$BINARY_SHA256 \
      io.ming-iot.role=dm
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY bin/dmsvr /app/dmsvr
COPY etc /app/etc
RUN mkdir -p /app/oss && chmod 0755 /app/dmsvr
ENTRYPOINT ["/app/dmsvr"]
