package conf

import (
	"fmt"
	"strings"

	oss "github.com/aliyun/aliyun-oss-go-sdk/oss"
)

type OssConf struct {
	OssType             string `json:",default=minio,options=minio|aliyun|local|aws"` //oss的类型
	AccessKeyID         string `json:",default=root,optional"`                        //账号
	AccessKeySecret     string `json:",default=password,optional"`                    //密码
	PublicBucketName    string `json:",default=ithings-public,optional"`              //公开桶的名称
	TemporaryBucketName string `json:",default=ithings-temporary,optional"`           //临时桶,30分钟有效期
	PrivateBucketName   string `json:",default=ithings-private,optional"`             //私有桶的名称
	Location            string `json:",default=localhost:9000,optional"`              // oss的地址,也叫endPoint
	Region              string `json:",optional"`                                     //区域,aws必填
	UseSSL              bool   `json:",optional"`                                     //是否使用ssl
	CustomHost          string `json:",default=/oss,env=OssCustomHost"`               //带上host的返回前缀,支持环境变量,如:http://127.0.0.1:7777/oss
	CustomPath          string `json:",default=/oss,optional"`                        //相对路径返回的前缀,minio及local模式使用
	ConnectTimeout      int64  `json:",default=10,optional"`                          //连接超时 单位秒
	ReadWriteTimeout    int64  `json:",default=10,optional"`                          //读写超时 单位秒
	StorePath           string `json:",optional,default=../oss"`                      //local模式,文件存储的位置
}

type LocalConf struct {
	OssConf
}

// minio本地存储配置
type MinioConf struct {
	OssConf
}

func (m MinioConf) GetEndPoint() string {
	return m.Location
}

// 阿里云oss配置
type AliYunConf struct {
	OssConf
}

// isCnameHost 判断 CustomHost 是否是可用于阿里云 CNAME 的合法域名。
// Minio/Local 模式下 CustomHost 可能是 "/oss" 或 "http://127.0.0.1:7777/oss" 这类路径/URL，
// 阿里云标准模式下应忽略，仅当配置为纯域名（如 xxx.example.com）时才作为 CNAME 使用。
func (a AliYunConf) isCnameHost() bool {
	return a.CustomHost != "" &&
		!strings.HasPrefix(a.CustomHost, "/") &&
		!strings.Contains(a.CustomHost, "://") &&
		!strings.Contains(a.CustomHost, "/") &&
		strings.Contains(a.CustomHost, ".")
}

func (a AliYunConf) GenClientOption() []oss.ClientOption {
	options := make([]oss.ClientOption, 0)
	options = append(options, oss.Timeout(a.ConnectTimeout, a.ReadWriteTimeout))
	if a.isCnameHost() {
		options = append(options, oss.UseCname(true))
	}
	return options
}

func (a AliYunConf) GetEndPoint() string {
	scheme := "https"
	if !a.UseSSL {
		scheme = "http"
	}
	if a.isCnameHost() {
		return fmt.Sprintf("%s://%s", scheme, a.CustomHost)
	}
	return fmt.Sprintf("%s://%s.aliyuncs.com", scheme, a.Location)
}

type AwsConf struct {
	OssConf
	RoleArn         string `json:",optional,env=OssRoleArn"` // 填写则启用 STS AssumeRole，如 arn:aws:iam::123456789012:role/OssRole，支持环境变量 OssRoleArn
	RoleSessionName string `json:",default=oss-session,optional"`
	SessionDuration int32  `json:",default=3600,optional"` // STS 临时凭证有效期（秒），最小 900，最大 43200
}

func (a AwsConf) GenClientOption() []oss.ClientOption {
	options := make([]oss.ClientOption, 0)
	options = append(options, oss.Timeout(a.ConnectTimeout, a.ReadWriteTimeout))
	if a.CustomHost != "" {
		options = append(options, oss.UseCname(true))
	}
	return options
}

func (a AwsConf) GetEndPoint() string {
	scheme := "https"
	if !a.UseSSL {
		scheme = "http"
	}
	if a.CustomHost == "" {
		return fmt.Sprintf("%s://%s.aliyuncs.com", scheme, a.Location)
	}
	return fmt.Sprintf("%s://%s", scheme, a.CustomHost)
}
