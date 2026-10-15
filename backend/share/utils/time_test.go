package utils

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestGetEndTime(t *testing.T) {
	location := time.FixedZone("UTC+8", 8*60*60)
	input := time.Date(2026, time.September, 4, 11, 41, 59, 123, location)
	type args struct {
		d time.Time
	}
	tests := []struct {
		name string
		args args
		want time.Time
	}{
		{
			name: "返回同一时区的次日零点",
			args: args{d: input},
			want: time.Date(2026, time.September, 5, 0, 0, 0, 0, location),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, GetEndTime(tt.args.d), "GetEndTime(%v)", tt.args.d)
		})
	}
}
