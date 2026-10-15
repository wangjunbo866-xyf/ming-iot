package utils

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestHmac(t *testing.T) {
	type args struct {
		sign   HmacType
		data   string
		secret []byte
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{args: args{sign: "hmacSha256", data: fmt.Sprintf("deviceName%vproductKey%v", "c5D542F00", "k0sr6A5BMbN"), secret: []byte("hxabHJVBJJyyUXDdu5Sa7xeLfyo=")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ret := Hmac(tt.args.sign, tt.args.data, tt.args.secret)
			fmt.Println(ret)
		})
	}
}

func TestMd5Map(t *testing.T) {
	type args struct {
		params map[string]any
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			args: args{params: map[string]any{"faefaeg": 234.234, "aaa": 123, "bbb": "faefgasdfaeg"}},
			want: "0ac7f04d08f071e0e67b469d016bd903",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, Md5Map(tt.args.params), "Md5Map(%v)", tt.args.params)
		})
	}
}
