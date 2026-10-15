package tools

import (
	"os"
	"strings"
	"testing"
)

const geoIPDebtIssue = "https://gitee.com/ming-industry/ming-iot/issues/IKD8AN"

func TestGetCityByIp(t *testing.T) {
	type args struct {
		ip string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "空地址", args: args{ip: ""}, want: ""},
		{name: "IPv4 回环地址", args: args{ip: "127.0.0.1"}, want: "内网IP"},
		{name: "IPv6 回环地址", args: args{ip: "[::1]"}, want: "内网IP"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetCityByIp(tt.args.ip); got != tt.want {
				t.Errorf("GetCityByIp() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetCityByIpExternal(t *testing.T) {
	if os.Getenv("MING_TEST_GEOIP") != "1" {
		t.Skipf("未设置 MING_TEST_GEOIP=1，跳过外部 GeoIP 服务测试；跟踪：%s", geoIPDebtIssue)
	}

	if got := strings.TrimSpace(GetCityByIp("115.193.200.155")); got == "" {
		t.Fatal("GetCityByIp() 调用外部 GeoIP 服务未返回城市")
	}
}
