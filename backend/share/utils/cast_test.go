package utils

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestInt64ToBStr(t *testing.T) {
	type args struct {
		num int64
		bit int
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "位数不足时保留完整二进制", args: args{num: 4323, bit: 12}, want: "1000011100011"},
		{name: "按指定位数左侧补零", args: args{num: 5, bit: 8}, want: "00000101"},
		{name: "零值按指定位数补零", args: args{num: 0, bit: 4}, want: "0000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, Int64ToBStr(tt.args.num, tt.args.bit), "Int64ToBStr(%v, %v)", tt.args.num, tt.args.bit)
		})
	}
}
