package utils

import (
	"database/sql"
	"encoding/base64"
	"time"

	"gitee.com/ming-iot/share/errors"
	"github.com/jinzhu/copier"
	"github.com/spf13/cast"
	wrapperspb "google.golang.org/protobuf/types/known/wrapperspb"
)

// converters 类型转换器列表，用于处理不同类型之间的转换
var converters []copier.TypeConverter

// TypeConverter 类型转换器别名
type (
	TypeConverter = copier.TypeConverter
)

// TimeStampUseMillisecond 时间戳是否使用毫秒级别
// true: 使用毫秒时间戳 (13位)
// false: 使用秒时间戳 (10位)
var TimeStampUseMillisecond bool

func init() {
	var (
		str1     *string
		str2     *wrapperspb.StringValue
		str3     sql.NullString
		str4     string
		str5     []byte
		strArr   []string
		int64Arr []int64
		strCopy  = []copier.TypeConverter{
			{SrcType: int64Arr, DstType: strArr, Fn: func(src interface{}) (dst interface{}, err error) {
				return cast.ToStringSliceE(src)
			}},
			{SrcType: strArr, DstType: int64Arr, Fn: func(src interface{}) (dst interface{}, err error) {
				in := src.([]string)
				var ret []int64
				for _, v := range in {
					ret = append(ret, cast.ToInt64(v))
				}
				return ret, nil
			}},
			{SrcType: str5, DstType: str4, Fn: func(src interface{}) (dst interface{}, err error) {
				return base64.StdEncoding.EncodeToString(src.([]byte)), nil
			}},
			{SrcType: str4, DstType: str5, Fn: func(src interface{}) (dst interface{}, err error) {
				decoded, err := base64.StdEncoding.DecodeString(src.(string))
				if err != nil {
					return nil, errors.Parameter.AddMsg("base64解析失败").AddDetail(err)
				}
				return decoded, nil
			}},
			{SrcType: str1, DstType: str2, Fn: func(src interface{}) (dst interface{}, err error) {
				return ToRpcNullString(src), nil
			}},
			{SrcType: str4, DstType: str2, Fn: func(src interface{}) (dst interface{}, err error) {
				return ToRpcNullString(src), nil
			}},
			{SrcType: str3, DstType: str2, Fn: func(src interface{}) (dst interface{}, err error) {
				return ToRpcNullString(src), nil
			}},
			{SrcType: str2, DstType: str1, Fn: func(src interface{}) (dst interface{}, err error) {
				return ToNullString(src.(*wrapperspb.StringValue)), nil
			}},
			{SrcType: str2, DstType: str4, Fn: func(src interface{}) (dst interface{}, err error) {
				return ToEmptyString(src.(*wrapperspb.StringValue)), nil
			}},
			{SrcType: str3, DstType: str4, Fn: func(src interface{}) (dst interface{}, err error) {
				return SqlToString(src.(sql.NullString)), nil
			}},
		}
	)
	converters = append(converters, strCopy...)
	var (
		int1 *wrapperspb.Int64Value
		int2 int64
		int3 *int64
		int4 sql.NullInt64
	)
	converters = append(converters,
		copier.TypeConverter{SrcType: str1, DstType: int1, Fn: func(src interface{}) (dst interface{}, err error) {
			return ToRpcNullInt64(src), nil
		}},
		copier.TypeConverter{SrcType: str4, DstType: int1, Fn: func(src interface{}) (dst interface{}, err error) {
			return ToRpcNullInt64(src), nil
		}},
		copier.TypeConverter{SrcType: str3, DstType: int1, Fn: func(src interface{}) (dst interface{}, err error) {
			return ToRpcNullInt64(src), nil
		}},
		copier.TypeConverter{SrcType: int3, DstType: int1, Fn: func(src interface{}) (dst interface{}, err error) {
			return ToRpcNullInt64(src), nil
		}},
		copier.TypeConverter{SrcType: int4, DstType: int1, Fn: func(src interface{}) (dst interface{}, err error) {
			return ToRpcNullInt64(src), nil
		}},
		copier.TypeConverter{SrcType: int2, DstType: int1, Fn: func(src interface{}) (dst interface{}, err error) {
			return ToRpcNullInt64(src), nil
		}},
		copier.TypeConverter{SrcType: int1, DstType: int3, Fn: func(src interface{}) (dst interface{}, err error) {
			return ToNullInt64(src.(*wrapperspb.Int64Value)), nil
		}},
		copier.TypeConverter{SrcType: int1, DstType: int2, Fn: func(src interface{}) (dst interface{}, err error) {
			return ToEmptyInt64(src.(*wrapperspb.Int64Value)), nil
		}})
	var (
		time1 time.Time
		time2 *time.Time
		time3 sql.NullTime
	)
	converters = append(converters,
		copier.TypeConverter{SrcType: time1, DstType: int2, Fn: func(src interface{}) (dst interface{}, err error) {
			t := src.(time.Time)
			if t.IsZero() {
				return int64(0), nil
			}
			if TimeStampUseMillisecond {
				return t.UnixMilli(), nil
			}
			return t.Unix(), nil
		}},
		copier.TypeConverter{SrcType: time2, DstType: int2, Fn: func(src interface{}) (dst interface{}, err error) {
			t := src.(*time.Time)
			if t == nil {
				return int64(0), nil
			}
			if TimeStampUseMillisecond {
				return t.UnixMilli(), nil
			}
			return t.Unix(), nil
		}},
		copier.TypeConverter{SrcType: time3, DstType: int2, Fn: func(src interface{}) (dst interface{}, err error) {
			t := src.(sql.NullTime)
			if t.Valid == false {
				return int64(0), nil
			}
			if TimeStampUseMillisecond {
				return t.Time.UnixMilli(), nil
			}
			return t.Time.Unix(), nil
		}},
		copier.TypeConverter{SrcType: int2, DstType: time1, Fn: func(src interface{}) (dst interface{}, err error) {
			in := src.(int64)
			if in == 0 {
				return time.Time{}, nil
			}
			if TimeStampUseMillisecond {
				return time.UnixMilli(in), nil
			}
			return time.Unix(in, 0), nil
		}},
		copier.TypeConverter{SrcType: int2, DstType: time2, Fn: func(src interface{}) (dst interface{}, err error) {
			if TimeStampUseMillisecond {
				return time.UnixMilli(src.(int64)), nil
			}
			return Int64ToTimex(src.(int64)), nil
		}},
		copier.TypeConverter{SrcType: int1, DstType: time3, Fn: func(src interface{}) (dst interface{}, err error) {
			in := src.(*wrapperspb.Int64Value)
			if in == nil || in.Value == 0 {
				return sql.NullTime{}, nil
			}
			if TimeStampUseMillisecond {
				return sql.NullTime{Valid: true, Time: time.UnixMilli(in.GetValue())}, nil
			}
			return ToNullTime2(src.(*wrapperspb.Int64Value)), nil
		}},
		copier.TypeConverter{SrcType: time3, DstType: int1, Fn: func(src interface{}) (dst interface{}, err error) {
			in := src.(*sql.NullTime)
			if !in.Valid {
				return nil, nil
			}
			if TimeStampUseMillisecond {
				return &wrapperspb.Int64Value{Value: in.Time.UnixMilli()}, nil
			}
			return TimeToNullInt(src.(sql.NullTime)), nil
		}},
		copier.TypeConverter{SrcType: int2, DstType: time3, Fn: func(src interface{}) (dst interface{}, err error) {
			in := src.(int64)
			if in == 0 {
				return sql.NullTime{}, nil
			}
			if TimeStampUseMillisecond {
				ret := time.UnixMilli(in)
				return sql.NullTime{Valid: true, Time: ret}, nil
			}
			return Int64ToSqlTime(src.(int64)), nil
		}})

	var (
		float1    *float32
		float2    *wrapperspb.FloatValue
		float3    sql.NullFloat64
		float4    float32
		floatCopy = []copier.TypeConverter{
			{SrcType: float1, DstType: float2, Fn: func(src interface{}) (dst interface{}, err error) {
				return ToRpcNullFloat32(src), nil
			}},
			{SrcType: float4, DstType: float2, Fn: func(src interface{}) (dst interface{}, err error) {
				return ToRpcNullFloat32(src), nil
			}},
			{SrcType: float3, DstType: float2, Fn: func(src interface{}) (dst interface{}, err error) {
				return ToRpcNullFloat32(src), nil
			}},
			{SrcType: float2, DstType: float1, Fn: func(src interface{}) (dst interface{}, err error) {
				return ToNullFloat32(src.(*wrapperspb.FloatValue)), nil
			}},
			{SrcType: float2, DstType: float4, Fn: func(src interface{}) (dst interface{}, err error) {
				return ToEmptyFloat32(src.(*wrapperspb.FloatValue)), nil
			}},
			{SrcType: float3, DstType: float4, Fn: func(src interface{}) (dst interface{}, err error) {
				return SqlToFloat32(src.(sql.NullFloat64)), nil
			}},
		}
	)
	converters = append(converters, floatCopy...)

	var (
		mapString map[string]string
		mapCopy   = []copier.TypeConverter{
			{SrcType: mapString, DstType: mapString, Fn: func(src interface{}) (dst interface{}, err error) {
				return src, nil
			}},
		}
	)
	converters = append(converters, mapCopy...)

}

// AddConverter 添加自定义类型转换器
// 参数:
//
//	in - 一个或多个 TypeConverter，用于定义源类型到目标类型的转换规则
//
// 使用场景:
//
//	当需要支持特殊类型之间的转换时，可以通过此函数注册自定义转换器
func AddConverter(in ...copier.TypeConverter) {
	converters = append(converters, in...)
}

// CopyE 拷贝对象，返回可能的错误
// 参数:
//
//	toValue - 目标对象（必须是指针类型）
//	fromValue - 源对象
//
// 返回:
//
//	error - 拷贝过程中发生的错误，nil 表示成功
//
// 特性:
//   - 深拷贝 (DeepCopy: true)
//   - 自动类型转换（使用预定义的 converters）
//
// 使用场景:
//
//	当需要处理拷贝错误时使用此函数
func CopyE(toValue interface{}, fromValue interface{}) (err error) {
	return copier.CopyWithOption(toValue, fromValue, copier.Option{
		DeepCopy:   true,
		Converters: converters,
	})
}

// Copy 拷贝对象，返回目标类型的指针
// 泛型参数:
//
//	toT - 目标类型
//
// 参数:
//
//	fromValue - 源对象
//
// 返回:
//
//	*toT - 拷贝后的目标对象指针，如果源对象为 nil 或拷贝失败则返回 nil
//
// 使用场景:
//
//	用于拷贝单个对象并返回指针类型
//	示例: userPB := utils.Copy[types.User](userRPC)
func Copy[toT any](fromValue any) *toT {
	var toValue toT
	if fromValue == nil {
		return nil
	}
	err := CopyE(&toValue, fromValue)
	if err != nil {
		return nil
	}
	return &toValue
}

// Copy2 拷贝对象，返回目标类型的值
// 泛型参数:
//
//	toT - 目标类型
//
// 参数:
//
//	fromValue - 源对象
//
// 返回:
//
//	toT - 拷贝后的目标对象值，如果源对象为 nil 或拷贝失败则返回零值
//
// 使用场景:
//
//	用于拷贝单个对象并返回值类型（非指针）
//	示例: user := utils.Copy2[types.User](userRPC)
func Copy2[toT any](fromValue any) toT {
	var toValue toT
	if fromValue == nil {
		return toValue
	}
	err := CopyE(&toValue, fromValue)
	if err != nil {
		return toValue
	}
	return toValue
}

// CopySlice 拷贝对象切片（源是指针切片，目标是指针切片）
// 泛型参数:
//
//	toT - 目标元素类型
//	fromT - 源元素类型（可省略，Go 会自动推导）
//
// 参数:
//
//	fromValue - 源对象切片 []*fromT
//
// 返回:
//
//	[]*toT - 拷贝后的目标对象切片，如果源切片为 nil 则返回 nil
//
// 使用场景:
//
//	用于将 RPC 返回的指针切片转换为 API 返回的指针切片
//	示例: apiUsers := utils.CopySlice[types.User](rpcUsers)
func CopySlice[toT any, fromT any](fromValue []*fromT) []*toT {
	if fromValue == nil {
		return nil
	}
	var ret []*toT
	for _, v := range fromValue {
		ret = append(ret, Copy[toT](v))
	}
	return ret
}

// CopySlice2 拷贝对象切片（源是值切片，目标是指针切片）
// 泛型参数:
//
//	toT - 目标元素类型
//	fromT - 源元素类型
//
// 参数:
//
//	fromValue - 源对象切片 []fromT
//
// 返回:
//
//	[]*toT - 拷贝后的目标对象指针切片，如果源切片为 nil 则返回 nil
//
// 使用场景:
//
//	用于将值切片转换为指针切片
//	示例: apiUsers := utils.CopySlice2[types.User](rpcUsers)
func CopySlice2[toT any, fromT any](fromValue []fromT) []*toT {
	if fromValue == nil {
		return nil
	}
	var ret []*toT
	for _, v := range fromValue {
		ret = append(ret, Copy[toT](v))
	}
	return ret
}

// CopySlice3 拷贝对象切片（源是指针切片，目标是值切片）
// 泛型参数:
//
//	toT - 目标元素类型
//	fromT - 源元素类型
//
// 参数:
//
//	fromValue - 源对象切片 []*fromT
//
// 返回:
//
//	[]toT - 拷贝后的目标对象值切片，如果源切片为 nil 则返回 nil
//
// 使用场景:
//
//	用于将指针切片转换为值切片
//	示例: apiUsers := utils.CopySlice3[types.User](rpcUsers)
func CopySlice3[toT any, fromT any](fromValue []*fromT) []toT {
	if fromValue == nil {
		return nil
	}
	var ret []toT
	for _, v := range fromValue {
		ret = append(ret, *Copy[toT](v))
	}
	return ret
}

// CopyMap 拷贝对象映射（值是指针，值是指针）
// 泛型参数:
//
//	toT - 目标值类型
//	fromT - 源值类型
//	keyT - 键类型（必须是可比较的）
//
// 参数:
//
//	fromValue - 源对象映射 map[keyT]*fromT
//
// 返回:
//
//	map[keyT]*toT - 拷贝后的目标对象映射，如果源映射为空则返回 nil
//
// 使用场景:
//
//	用于拷贝 map，其中源和值的值都是指针类型
//	示例: apiMap := utils.CopyMap[types.User](rpcMap)
func CopyMap[toT any, fromT any, keyT comparable](fromValue map[keyT]*fromT) map[keyT]*toT {
	if len(fromValue) == 0 {
		return nil
	}
	var ret = map[keyT]*toT{}
	for k, v := range fromValue {
		ret[k] = Copy[toT](v)
	}
	return ret
}

// CopyMap2 拷贝对象映射（值是值，值是指针）
// 泛型参数:
//
//	toT - 目标值类型
//	fromT - 源值类型
//	keyT - 键类型（必须是可比较的）
//
// 参数:
//
//	fromValue - 源对象映射 map[keyT]fromT
//
// 返回:
//
//	map[keyT]*toT - 拷贝后的目标对象映射，如果源映射为空则返回 nil
//
// 使用场景:
//
//	用于将值映射转换为指针映射
//	示例: apiMap := utils.CopyMap2[types.User](rpcMap)
func CopyMap2[toT any, fromT any, keyT comparable](fromValue map[keyT]fromT) map[keyT]*toT {
	if len(fromValue) == 0 {
		return nil
	}
	var ret = map[keyT]*toT{}
	for k, v := range fromValue {
		ret[k] = Copy[toT](&v)
	}
	return ret
}

// CopyMap3 拷贝对象映射（值是指针，值是值）
// 泛型参数:
//
//	toT - 目标值类型
//	fromT - 源值类型
//	keyT - 键类型（必须是可比较的）
//
// 参数:
//
//	fromValue - 源对象映射 map[keyT]*fromT
//
// 返回:
//
//	map[keyT]toT - 拷贝后的目标对象映射，如果源映射为空则返回 nil
//
// 使用场景:
//
//	用于将指针映射转换为值映射
//	示例: apiMap := utils.CopyMap3[types.User](rpcMap)
func CopyMap3[toT any, fromT any, keyT comparable](fromValue map[keyT]*fromT) map[keyT]toT {
	if len(fromValue) == 0 {
		return nil
	}
	var ret = map[keyT]toT{}
	for k, v := range fromValue {
		ret[k] = Copy2[toT](v)
	}
	return ret
}
