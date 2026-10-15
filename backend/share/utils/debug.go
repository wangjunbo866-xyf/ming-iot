/*
AI 调试日志工具 - 输出 JSON Lines 格式日志，便于 Claude 快速验证代码运行结果。

环境变量:
  - AI_DEBUG: 设置为 "true" 或 "1" 启用调试日志
  - AI_DEBUG_LOG_FILE: 日志文件路径（默认 .debug.log）

使用示例:

	// 格式化输出
	utils.DebugLog("mqtt.auth", "设备认证 productID=%s", productID)

	// 结构化数据
	utils.DebugLogData("device.down", "下行指令", map[string]any{"topic": topic})

	// 带 ctx 版本（支持链路追踪）
	utils.DebugLogCtx(ctx, "api.request", "处理请求 path=%s", path)

日志过滤 (jq):

	cat .debug.log | jq .                                    # 查看所有日志
	cat .debug.log | jq 'select(.code=="mqtt.auth")'         # 按 code 精确过滤
	cat .debug.log | jq 'select(.code | startswith("mqtt"))' # 按 code 前缀过滤
	cat .debug.log | jq 'select(.caller | contains("dmsvr"))'# 按调用位置过滤
	cat .debug.log | jq 'select(.trace=="884c0fd9...")'      # 按链路追踪 ID 过滤
	cat .debug.log | jq '{code, msg, data}'                  # 只提取特定字段

常用 code 分类: mqtt.*, nats.*, device.*, rpc.*, db.*, api.*
*/
package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// debugEntry AI 调试日志条目
type debugEntry struct {
	Ts     string         `json:"ts"`
	Code   string         `json:"code"`
	Caller string         `json:"caller"`
	Trace  string         `json:"trace,omitempty"`
	Span   string         `json:"span,omitempty"`
	Msg    string         `json:"msg"`
	Data   map[string]any `json:"data,omitempty"`
}

var (
	debugFile *os.File
	debugOnce sync.Once
	debugMu   sync.Mutex
)

// initDebugLog 初始化调试日志文件
func initDebugLog() {
	debugOnce.Do(func() {
		env := os.Getenv("AI_DEBUG")
		if env != "true" && env != "1" {
			return
		}

		path := os.Getenv("AI_DEBUG_LOG_FILE")
		if path == "" {
			path = ".debug.log"
		}

		dir := filepath.Dir(path)
		if dir != "" && dir != "." {
			os.MkdirAll(dir, 0755)
		}

		f, _ := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		debugFile = f
	})
}

// debugCaller 获取调用位置
func debugCaller() string {
	pcs := [13]uintptr{}
	length := runtime.Callers(4, pcs[:])
	frames := runtime.CallersFrames(pcs[:length])
	frame, _ := frames.Next()
	return prettyCaller(frame.File, frame.Line)
}

func writeDebugLog(code, msg string, data map[string]any) {
	writeDebugLogCtx(context.Background(), code, msg, data)
}

func writeDebugLogCtx(ctx context.Context, code, msg string, data map[string]any) {
	initDebugLog()
	if debugFile == nil {
		return
	}

	spanCtx := trace.SpanContextFromContext(ctx)
	var traceID, spanID string
	if spanCtx.HasTraceID() {
		traceID = spanCtx.TraceID().String()
		spanID = spanCtx.SpanID().String()
	}

	entry := debugEntry{
		Ts:     time.Now().Format("15:04:05.000"),
		Code:   code,
		Caller: debugCaller(),
		Trace:  traceID,
		Span:   spanID,
		Msg:    msg,
		Data:   data,
	}
	line, _ := json.Marshal(entry)
	debugMu.Lock()
	debugFile.Write(line)
	debugFile.WriteString("\n")
	debugMu.Unlock()
}

// DebugLog 格式化输出（无 ctx）
func DebugLog(code string, format string, a ...any) {
	writeDebugLog(code, fmt.Sprintf(format, a...), nil)
}

// DebugLogCtx 格式化输出（带 ctx，支持链路追踪）
func DebugLogCtx(ctx context.Context, code string, format string, a ...any) {
	writeDebugLogCtx(ctx, code, fmt.Sprintf(format, a...), nil)
}

// DebugLogData 结构化数据输出
func DebugLogData(code string, msg string, data map[string]any) {
	writeDebugLog(code, msg, data)
}

// DebugLogDataCtx 结构化数据输出（带 ctx）
func DebugLogDataCtx(ctx context.Context, code string, msg string, data map[string]any) {
	writeDebugLogCtx(ctx, code, msg, data)
}

// DebugLogMsg 简单消息输出
func DebugLogMsg(code string, msg string) {
	writeDebugLog(code, msg, nil)
}

// DebugLogMsgCtx 简单消息输出（带 ctx）
func DebugLogMsgCtx(ctx context.Context, code string, msg string) {
	writeDebugLogCtx(ctx, code, msg, nil)
}
