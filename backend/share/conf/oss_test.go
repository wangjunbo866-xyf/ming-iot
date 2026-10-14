package conf

import (
	"testing"
)

func TestAliYunConf_GetEndPoint(t *testing.T) {
	tests := []struct {
		name      string
		conf      AliYunConf
		want      string
		wantCname bool
	}{
		{
			name: "默认 CustomHost /oss 应使用标准地域节点",
			conf: AliYunConf{OssConf: OssConf{Location: "oss-cn-wulanchabu", UseSSL: false, CustomHost: "/oss"}},
			want: "http://oss-cn-wulanchabu.aliyuncs.com",
		},
		{
			name: "空 CustomHost 应使用标准地域节点",
			conf: AliYunConf{OssConf: OssConf{Location: "oss-cn-wulanchabu", UseSSL: true, CustomHost: ""}},
			want: "https://oss-cn-wulanchabu.aliyuncs.com",
		},
		{
			name: "合法 CNAME 域名应作为 endpoint",
			conf: AliYunConf{OssConf: OssConf{Location: "oss-cn-wulanchabu", UseSSL: false, CustomHost: "ithings-temporary.cn-wulanchabu.taihangqzs.cn"}},
			want: "http://ithings-temporary.cn-wulanchabu.taihangqzs.cn",
		},
		{
			name: "bucket 域名应作为 CNAME endpoint",
			conf: AliYunConf{OssConf: OssConf{Location: "oss-cn-wulanchabu", UseSSL: true, CustomHost: "ithings-temporary.oss-cn-wulanchabu.aliyuncs.com"}},
			want: "https://ithings-temporary.oss-cn-wulanchabu.aliyuncs.com",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.conf.GetEndPoint(); got != tt.want {
				t.Errorf("GetEndPoint() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAliYunConf_isCnameHost(t *testing.T) {
	tests := []struct {
		name string
		host string
		want bool
	}{
		{"默认 /oss", "/oss", false},
		{"空字符串", "", false},
		{"相对路径", "/oss/prefix", false},
		{"合法 CNAME", "ithings-temporary.cn-wulanchabu.taihangqzs.cn", true},
		{"bucket 域名", "ithings-temporary.oss-cn-wulanchabu.aliyuncs.com", true},
		{"带 scheme 不应作为 CNAME", "http://example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := AliYunConf{OssConf: OssConf{CustomHost: tt.host}}
			if got := c.isCnameHost(); got != tt.want {
				t.Errorf("isCnameHost(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}
