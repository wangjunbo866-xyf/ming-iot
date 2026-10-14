package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"gitee.com/ming-iot/share/utils"
	"github.com/parnurzeal/gorequest"
	"github.com/zeromicro/go-zero/core/logx"
	"time"
)

type HolidayType int64

const (
	HolidayWorkDay  = 0 //工作日或者补班
	HolidayWeekend  = 1 //周末
	HolidayFestival = 2 //节日
)

type HolidayInfo struct {
	Holiday HolidayType `json:"holiday"`
	Wage    int         `json:"wage"` //薪资倍数
}

type holidayDetail struct {
	Holiday bool   `json:"holiday"`
	Name    string `json:"name"`
	Wage    int    `json:"wage"`
	Date    string `json:"date"`
	Rest    int    `json:"rest"`
	After   bool   `json:"after"`  // 是否为节后补班
	Before  bool   `json:"before"` // 是否为节前补班
}

type holidayResp struct {
	Code    int                      `json:"code"`
	Holiday map[string]holidayDetail `json:"holiday"`
}

func GetHoliday(ctx context.Context, t time.Time) (ret *HolidayInfo, err error) {
	var year, month, day = t.Date()
	var key = fmt.Sprintf("share:tools:holiday:v2:%04d-%02d-%02d", year, month, day)
	val, err := store.Get(key)
	if !(err != nil || val == "") {
		err := json.Unmarshal([]byte(val), &ret)
		return ret, err
	}

	var (
		holidayMap = initHoliday(year, month)
		gReq       = gorequest.New().Retry(5, time.Second*1)
		data       holidayResp
	)
	func() {
		//官网: http://timor.tech/api/holiday
		_, body, errs := gReq.Get(fmt.Sprintf("http://timor.tech/api/holiday/year/%04d-%02d", year, month)).
			Set("User-Agent", "iThings").EndStruct(&data) //不加User-Agent会被拦截
		fmt.Println(string(body))
		if errs != nil {
			logx.WithContext(ctx).Error(errs)
			return
		}
		for k, v := range data.Holiday {
			holiday := HolidayWeekend
			if v.Holiday && v.Wage >= 2 {
				// wage >= 2 才认为是放假的法定节假日或调休日（wage=3 法定三倍，wage=2 调休两倍）
				holiday = HolidayFestival
			} else {
				// holiday=false（补班）或 wage=1（非放假节日）都视为工作日
				holiday = HolidayWorkDay
			}
			holidayMap[fmt.Sprintf("%04d-%v", year, k)] = &HolidayInfo{
				Holiday: HolidayType(holiday),
				Wage:    v.Wage,
			}
		}
	}()
	for k, v := range holidayMap {
		var dayKey = fmt.Sprintf("share:tools:holiday:v2:%s", k)
		valByte, _ := json.Marshal(v)
		store.SetexCtx(ctx, dayKey, string(valByte), 60*60*24*7) //保留7天
		if dayKey == key {
			ret = v
		}
	}
	return
}
func initHoliday(year int, month time.Month) map[string]*HolidayInfo {
	var retMap = map[string]*HolidayInfo{}
	days := utils.GetMonthDays(year, month)
	for day := 1; day <= days; day++ {
		t := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
		info := HolidayInfo{
			Holiday: HolidayWorkDay,
			Wage:    1,
		}
		if utils.SliceIn(t.Weekday(), time.Sunday, time.Saturday) {
			info.Holiday = HolidayWeekend
		}
		retMap[fmt.Sprintf("%04d-%02d-%02d", year, month, day)] = &info
	}
	return retMap
}
