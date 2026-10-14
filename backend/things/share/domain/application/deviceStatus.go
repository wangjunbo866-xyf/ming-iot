package application

import (
	"gitee.com/ming-iot/share/def"
	"gitee.com/ming-iot/things/share/devices"
)

// 连接和断连消息信息
type ConnectMsg struct {
	Device    devices.Core   `json:"device"`
	Status    def.ConnStatus `json:"status"`
	Timestamp int64          `json:"timestamp,string"` //毫秒时间戳
}

func (c ConnectMsg) GenSerial() string {
	return ""
}
