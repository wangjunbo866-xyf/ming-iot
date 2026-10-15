package utils

import (
	"context"
	"os"
	"sync"
	"testing"
)

func TestDebugLog(t *testing.T) {
	// 重置 sync.Once（测试环境需要）
	debugOnce = sync.Once{}
	debugFile = nil

	// 设置环境变量
	os.Setenv("AI_DEBUG", "true")
	os.Setenv("AI_DEBUG_LOG_FILE", "/tmp/test_debug.log")
	defer os.Unsetenv("AI_DEBUG")
	defer os.Unsetenv("AI_DEBUG_LOG_FILE")

	// 测试格式化输出
	DebugLog("test.format", "测试格式化 value=%d", 123)

	// 测试结构化输出
	DebugLogData("test.data", "测试结构化", map[string]any{
		"key": "value",
		"num": 456,
	})

	// 测试简单消息
	DebugLogMsg("test.msg", "测试简单消息")

	// 测试带 ctx 版本
	ctx := context.Background()
	DebugLogCtx(ctx, "test.ctx", "带ctx测试")

	// 验证文件存在
	if _, err := os.Stat("/tmp/test_debug.log"); os.IsNotExist(err) {
		t.Error("日志文件未创建")
	}

	// 读取并验证内容
	content, err := os.ReadFile("/tmp/test_debug.log")
	if err != nil {
		t.Errorf("读取日志文件失败: %v", err)
	}

	if len(content) == 0 {
		t.Error("日志文件为空")
	}

	// 清理
	os.Remove("/tmp/test_debug.log")
}
