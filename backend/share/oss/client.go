package oss

import (
	"context"
	"gitee.com/ming-iot/share/errors"
	"sync"

	"gitee.com/ming-iot/share/conf"
	"gitee.com/ming-iot/share/oss/common"
)

type Client struct {
	Handle
}

var (
	client *Client
	mu     sync.Mutex
)

// NewOssClient 创建或返回已有的 OSS 客户端（线程安全，失败后允许重试）
func NewOssClient(c conf.OssConf) (cli *Client, err error) {
	if client != nil {
		return client, nil
	}
	mu.Lock()
	defer mu.Unlock()
	// double-check: 锁内再次检查，避免并发重复初始化
	if client != nil {
		return client, nil
	}
	ossManager, er := newOssManager(c)
	if er != nil {
		return nil, errors.Parameter.AddMsgf("oss 初始化失败 err:%v", er)
	}
	client = &Client{ossManager}
	return client, nil
}

type OpOption func(*common.OptionKv)

func (c *Client) getDefaultOption(ctx context.Context) OpOption {
	return func(option *common.OptionKv) {
		option.SetHttpParams("x-process", "xxxxx")
	}
}
