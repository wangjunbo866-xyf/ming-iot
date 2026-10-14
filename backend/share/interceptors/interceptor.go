package interceptors

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/utils"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/timex"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const defaultSlowThreshold = time.Second

func Error(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	resp, err := handler(ctx, req)
	if err != nil {
		// 错误时打印响应体，便于定位问题；成功响应不打印
		body := utils.MarshalNoErr(resp)
		if len(body) > 1024 {
			body = body[:1024]
		}
		logx.WithContext(ctx).Errorf("err=%s resp=%v", errors.Fmt(err).Error(), body)
	}
	// 成功响应不再打印，避免大量 resp={} 日志
	var acceptLanguage string
	if uc := ctxs.GetUserCtx(ctx); uc != nil {
		acceptLanguage = uc.AcceptLanguage
	}
	err = errors.ToRpc(err, acceptLanguage)
	return resp, err
}

// StatInterceptor 打印 RPC 调用摘要，替代 go-zero 默认 stat 拦截器。
// 保留 method/cost/status/请求体/请求体大小/调用方等关键信息；成功响应不打印，错误响应在 Error 拦截器中打印。
func StatInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := timex.Now()
		resp, err := handler(ctx, req)
		duration := timex.Since(start)

		var reqSize, respSize int
		var reqBody string
		if m, ok := req.(proto.Message); ok && m != nil {
			reqSize = proto.Size(m)
			reqBody = utils.MarshalNoErr(m)
			if len(reqBody) > 1024 {
				reqBody = reqBody[:1024]
			}
		}
		if m, ok := resp.(proto.Message); ok && m != nil {
			respSize = proto.Size(m)
		}

		var userID int64
		var tenantCode string
		if uc := ctxs.GetUserCtx(ctx); uc != nil {
			userID = uc.UserID
			tenantCode = uc.TenantCode
		}

		costMs := float64(duration.Microseconds()) / 1000
		logx.WithContext(ctx).Infow("rpc",
			logx.Field("method", info.FullMethod),
			logx.Field("cost", costMs),
			logx.Field("status", status.Code(err).String()),
			logx.Field("req", reqBody),
			logx.Field("reqSize", reqSize),
			logx.Field("respSize", respSize),
			logx.Field("userID", userID),
			logx.Field("tenant", tenantCode),
		)
		if duration > defaultSlowThreshold {
			logx.WithContext(ctx).Sloww("rpc slowcall",
				logx.Field("method", info.FullMethod),
				logx.Field("cost", costMs),
				logx.Field("status", status.Code(err).String()),
				logx.Field("req", reqBody),
				logx.Field("reqSize", reqSize),
				logx.Field("respSize", respSize),
				logx.Field("userID", userID),
				logx.Field("tenant", tenantCode),
			)
		}
		return resp, err
	}
}

func Ctxs(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	ctx2 := func() context.Context {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return ctx
		}
		info := md[ctxs.UserInfoKey]
		if len(info) == 0 {
			return ctx
		}
		var val ctxs.UserCtx
		str, err := base64.StdEncoding.DecodeString(info[0])
		if err != nil {
			return ctx
		}
		if err := json.Unmarshal(str, &val); err != nil {
			return ctx
		}
		return ctxs.SetUserCtx(ctx, &val)
	}()
	resp, err := handler(ctx2, req)
	return resp, err
}
