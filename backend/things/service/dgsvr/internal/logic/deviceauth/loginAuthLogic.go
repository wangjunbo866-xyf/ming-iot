package deviceauthlogic

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/errors"
	"gitee.com/ming-iot/share/utils"
	"gitee.com/ming-iot/things/service/dgsvr/internal/svc"
	"gitee.com/ming-iot/things/service/dgsvr/pb/dg"
	"gitee.com/ming-iot/things/share/devices"
	"gitee.com/ming-iot/things/share/domain/deviceAuth"
	"github.com/zeromicro/go-zero/core/logx"
)

type LoginAuthLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

var clientCert string = `-----BEGIN CERTIFICATE-----
MIIC3zCCAcegAwIBAgIBAjANBgkqhkiG9w0BAQsFADATMREwDwYDVQQDEwhNeVRl
c3RDQTAeFw0xNjEyMjYwMzA4MjNaFw0xNzEyMjYwMzA4MjNaMCIxDzANBgNVBAMM
BmNsb3VkMzEPMA0GA1UECgwGY2xpZW50MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A
MIIBCgKCAQEA40Q+bMUjxCOdDcdC2jZaX8HuNCdm6Mu1rgj8ZfyTJIzsKtv00LYd
xfdhlNFj1uq8wi/zK/cB95wBpG1Loo/WicqSP2G/A7aPnzIBPj3zzP7HdyM5EaHW
zDWLzK+f0+MmAsrp7UW/zBR5O+ScnmIWm2H7KJY36dJPKllzzw+R6a4eJ6vthBcm
nueIYrhdXnunaWzkWQqAWlSZCzD8/MfTkgAPYW7OoS6aAQugTBzhHRo1meOVIT7u
y+hmZE4kE8V98Iy1rGPV5Uz/1vSEJziJGvQkyVr3gcAv5DwLWnyX1vOXKIKOgRAz
VYB4tbfEP5tQ0jimG5ErftF/sGYFNTRslQIDAQABoy8wLTAJBgNVHRMEAjAAMAsG
A1UdDwQEAwIHgDATBgNVHSUEDDAKBggrBgEFBQcDAjANBgkqhkiG9w0BAQsFAAOC
AQEAMtVzZdj1y/TLxP7KZcqkd/Z/vdW6moo12tahDHR4vPq0NdGaHADRfZHbCBmb
JEI9Qz3CKSdKZRZ4/A3ui/ZltbvkCao9ilmhQXXDT3Yz5hxk5ZBC9+Zs1IZmrsis
Qg/cdLUx4+ei/eR0OgWyC5D9AKNzshQExKBGGojedb98VcuS5ccJKrq0kVzZ/BZQ
k1EswNC9ifKBcqPIO1rTD9T3PB7dv9ZRpxwslmgYWWsqQu9x/dnOPEHJ1yXr7KJh
47NP7OrtG21la8EcAtjA3GXDjiZ+tXIR1RMbx1iuJAQPBeddWJCPbyofdNRL59BQ
caLVQ77r0hpvnNpkafa5QptyfQ==
-----END CERTIFICATE-----`

var x509Cert *x509.Certificate

func init() {
	clientCertBlock, _ := pem.Decode([]byte(clientCert))
	if clientCertBlock != nil {
		if parsedCert, err := x509.ParseCertificate(clientCertBlock.Bytes); err == nil {
			x509Cert = parsedCert
		}
	}
}
func NewLoginAuthLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LoginAuthLogic {
	return &LoginAuthLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *LoginAuthLogic) subscribe(clientID string, productID, deviceName string, topics ...string) {
	ctxs.GoNewCtx(l.ctx, func(ctx context.Context) {
		var newTopics []string
		for _, t := range topics {
			newTopics = append(newTopics, fmt.Sprintf(t, productID, deviceName))
		}
		err := l.svcCtx.MqttClient.SetClientMutSub(ctx, clientID, newTopics, 1)
		if err != nil {
			logx.WithContext(ctx).Error(err)
			return
		}
	})
}

var topics = []string{
	"$thing/down/property/%s/%s",
	"$thing/down/event/%s/%s",
	"$thing/down/action/%s/%s",
	"$ota/down/upgrade/%s/%s",
}

// 设备登录认证
func (l *LoginAuthLogic) LoginAuth(in *dg.LoginAuthReq) (*dg.Response, error) {
	l.Infof("%s req=%+v", utils.FuncName(), in)
	if len(in.Certificate) > 0 && x509Cert != nil {
		if bytes.Equal(in.Certificate, x509Cert.Signature) {
			l.Error("it is same")
		}
		l.Errorf("cert len=%d signature len=%d",
			len(x509Cert.Raw), len(x509Cert.Signature))
	}
	inLg, err := deviceAuth.GetClientIDInfo(in.ClientID)
	if err != nil {
		return nil, err
	}
	if inLg.IsNeedRegister { //如果是需要注册的直接放过
		//l.subscribe(in.ClientID, inLg.ProductID, inLg.DeviceName, "$ext/down/register/%s/%s")
		return &dg.Response{}, nil
	}
	//生成 MQTT 的 username 部分, 格式为 ${clientid};${sdkappid};${connid};${expiry}
	lg, err := deviceAuth.GetLoginDevice(in.Username)
	if err != nil {
		return nil, err
	}

	if lg.ProductID != inLg.ProductID || lg.DeviceName != inLg.DeviceName {
		return nil, errors.Parameter.AddDetail("userName'clientID not equal real client id")
	}
	//if lg.Expiry < time.Now().Unix() {
	//	return nil, errors.SignatureExpired
	//}

	di, err := l.svcCtx.DeviceCache.GetData(l.ctx, devices.Core{
		ProductID:  lg.ProductID,
		DeviceName: lg.DeviceName,
	})
	if err != nil {
		return nil, err
	}
	pwd, err := deviceAuth.NewPwdInfoWithPwd(in.Password)
	if err != nil {
		return nil, err
	}
	err = pwd.CmpPwd(in.Username, di.Secret)
	if err != nil {
		return nil, err
	}
	//l.subscribe(in.ClientID, inLg.ProductID, inLg.DeviceName, topics...)
	return &dg.Response{}, nil
}
