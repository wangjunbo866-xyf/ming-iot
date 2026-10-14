package oauth2

import (
	"context"
	"gitee.com/ming-iot/share/conf"
	"golang.org/x/oauth2"
	"net/url"
	"reflect"
	"testing"
)

func TestGithubClient_GetAuthCodeURL(t *testing.T) {
	type fields struct {
		config *oauth2.Config
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &GithubClient{
				config: tt.fields.config,
			}
			if got := g.GetAuthCodeURL(); got != tt.want {
				t.Errorf("GetAuthCodeURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGithubClient_GetUserInfo(t *testing.T) {
	type fields struct {
		config *oauth2.Config
	}
	type args struct {
		ctx  context.Context
		code string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *GitHubUser
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &GithubClient{
				config: tt.fields.config,
			}
			got, err := g.GetUserInfo(tt.args.ctx, tt.args.code)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetUserInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetUserInfo() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewGithub(t *testing.T) {
	ctx := context.Background()
	c := NewGithub(ctx, &conf.ThirdConf{
		AppID:     "test-client-id",
		AppSecret: "test-only",
	})
	u, err := url.Parse(c.GetAuthCodeURL())
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "github.com" || u.Query().Get("client_id") != "test-client-id" || u.Query().Get("response_type") != "code" {
		t.Fatalf("unexpected authorization URL: %s", u)
	}
}
