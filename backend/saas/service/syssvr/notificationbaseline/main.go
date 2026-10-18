// 本文件通过真实 gRPC 验证隔离 syssvr 通知持久化，禁止配置远程目标。
package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"gitee.com/ming-iot/saas/service/syssvr/internal/repo/relationDB"
	"gitee.com/ming-iot/saas/service/syssvr/pb/sys"
	"gitee.com/ming-iot/share/ctxs"
	"gitee.com/ming-iot/share/def"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// main 仅接受本隔离 profile 的固定入口。
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run 等待真实 RPC 并新增唯一标记数据，历史记录始终保留。
func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "syssvr:9180", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return fmt.Errorf("RPC readiness: %w", err)
	}
	defer conn.Close()
	if len(os.Args) == 2 && os.Args[1] == "ready" {
		return nil
	}
	if len(os.Args) != 2 || os.Args[1] != "verify" {
		return fmt.Errorf("usage: probe ready|verify")
	}
	db, err := gorm.Open(postgres.Open("host=postgres user=notification password=notification-local-only dbname=notification port=5432 sslmode=disable"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	root := ctxs.WithRoot(ctx)
	db = db.WithContext(root)
	mark := fmt.Sprintf("nb-%d", time.Now().UnixNano())
	var previous int64
	if err := db.Table("sys_message_info").Where("str1 LIKE ?", "nb-%").Count(&previous).Error; err != nil {
		return err
	}
	user := relationDB.SysUserInfo{NickName: "基线昵称", UserName: sql.NullString{String: mark, Valid: true}, Password: "disabled", Tags: map[string]string{}, PubTags: map[string]string{}}
	if err := db.Create(&user).Error; err != nil {
		return fmt.Errorf("seed user: %w", err)
	}
	// 模板使用已有 ruleScene 配置，每次创建独立模板避免覆盖既有样例。
	template := relationDB.SysNotifyTemplate{Name: mark, NotifyCode: "ruleScene", Type: "message", TemplateCode: mark, Subject: "{{.title}}", Body: "{{.body}}"}
	if err := db.Create(&template).Error; err != nil {
		return fmt.Errorf("seed template: %w", err)
	}
	uc, err := json.Marshal(ctxs.GetUserCtx(root))
	if err != nil {
		return err
	}
	rpcCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs(ctxs.UserInfoKey, base64.StdEncoding.EncodeToString(uc)))
	client := sys.NewNotifyManageClient(conn)
	// cases 对照 current-v1 CALL-SITES；manual 不新增昵称行，auto 与按键保留各自边界。
	cases := []struct {
		name, kind, typ, body, expected, nick, account string
		uid                                            int64
	}{
		{"manual", "manual", "message", "打开客厅灯", "打开客厅灯", "基线昵称", mark, user.UserID},
		{"auto", "auto", "message", "打开客厅灯", "系统自动触发\n打开客厅灯", "系统自动", "", 0},
		{"button-message", "deviceButton", "message", "开灯\n打开客厅灯", mark + "触发了打开客厅灯\n开灯\n打开客厅灯", "基线昵称", mark, user.UserID},
		{"button-system", "deviceButton", "systemNotice", "开灯\n打开客厅灯", mark + "触发了打开客厅灯", "基线昵称", mark, user.UserID},
	}
	for _, tc := range cases {
		title := mark + "/" + tc.name
		params := map[string]string{"title": title, "body": tc.body, "triggerType": tc.kind, "triggerUserId": fmt.Sprint(tc.uid)}
		_, err = client.NotifyConfigSend(rpcCtx, &sys.NotifyConfigSendReq{UserIDs: []int64{user.UserID}, NotifyCode: "ruleScene", Type: tc.typ, TemplateID: template.ID, Params: params, Str1: mark})
		if err != nil {
			return fmt.Errorf("%s RPC: %w", tc.name, err)
		}
		var rows []relationDB.SysMessageInfo
		if err = db.Where("str1 = ? AND subject = ?", mark, title).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != 1 {
			return fmt.Errorf("%s message count=%d", tc.name, len(rows))
		}
		m := rows[0]
		if m.Body != tc.expected || m.Subject != title || m.TriggerType != tc.kind || m.TriggerUserID != tc.uid || m.TriggerUserNick != tc.nick || m.TriggerUserAccount != tc.account || m.NotifyType != tc.typ || string(m.TenantCode) != def.TenantCodeDefault {
			return fmt.Errorf("%s persisted fields mismatch: %+v", tc.name, m)
		}
		var recipients []relationDB.SysUserMessage
		if err = db.Where("message_id = ?", m.ID).Find(&recipients).Error; err != nil {
			return err
		}
		if len(recipients) != 1 || recipients[0].UserID != user.UserID || string(recipients[0].TenantCode) != def.TenantCodeDefault {
			return fmt.Errorf("%s recipient mismatch", tc.name)
		}
		fmt.Printf("PASS case=%s message_id=%d recipient=%d body=%q\n", tc.name, m.ID, user.UserID, m.Body)
	}
	fmt.Printf("PASS notification_rpc_persistence marker=%s previous_smoke_records=%d new_records=%d external_delivery=disabled eventbus_in_notification_path=false\n", mark, previous, len(cases))
	return nil
}
