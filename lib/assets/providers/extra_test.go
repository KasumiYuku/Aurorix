package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KasumiYuku/Aurorix/lib/assets"
)

func TestCNBSkipsProbe(t *testing.T) {
	if !assets.ProbeSkippedByDefault("cnb") {
		t.Fatal("cnb 应默认跳过上传后探测")
	}
}

func TestCNBUpload(t *testing.T) {
	var putBody []byte
	var putAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			if r.Header.Get("Authorization") != "Bearer tok" {
				t.Errorf("申请上传地址的 Authorization 不对: %q", r.Header.Get("Authorization"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"upload_url": "http://" + r.Host + "/put",
				"token":      "puttok",
				"assets":     map[string]any{"url": "/u/r/-/assets/a.png", "path": "u/r/-/assets/a.png"},
			})
		case r.Method == http.MethodPut:
			putAuth = r.Header.Get("Authorization")
			putBody, _ = io.ReadAll(r.Body)
		default:
			t.Errorf("意外请求: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	cl := assets.NewClient(5)
	p, err := assets.Instantiate("cnb", cl, map[string]any{
		"token": "tok", "defaultRepo": "u/r", "baseUrl": srv.URL, "autodelete": float64(0),
	})
	if err != nil {
		t.Fatalf("实例化失败: %v", err)
	}
	payload := []byte("fake-image-bytes")
	url, err := p.Upload(context.Background(), assets.ProviderInput{
		Buffer: payload, Filename: "a.png", MimeType: "image/png",
	})
	if err != nil {
		t.Fatalf("上传失败: %v", err)
	}
	if url != "https://cnb.cool/u/r/-/assets/a.png" {
		t.Fatalf("返回 URL = %q", url)
	}
	if putAuth != "Bearer puttok" {
		t.Fatalf("直传 Authorization = %q", putAuth)
	}
	if string(putBody) != string(payload) {
		t.Fatalf("直传字节不一致: %q", putBody)
	}
}

func TestMihoyoEqFields(t *testing.T) {
	policy := base64.StdEncoding.EncodeToString([]byte(`{"conditions":[` +
		`["content-length-range",0,100],` +
		`["eq","$x-oss-forbid-overwrite","true"],` +
		`["eq","$key","abc"],` +
		`["starts-with","$name",""]` +
		`]}`))
	got := mihoyoEqFields(policy)
	if got["x-oss-forbid-overwrite"] != "true" || got["key"] != "abc" {
		t.Fatalf("eq 字段解析不对: %v", got)
	}
	if _, ok := got["content-length-range"]; ok {
		t.Fatalf("非 eq 条件不应出现: %v", got)
	}
}

func TestIntOr(t *testing.T) {
	cases := []struct {
		cfg  map[string]any
		want int
	}{
		{map[string]any{"n": float64(42)}, 42},
		{map[string]any{"n": "7"}, 7},
		{map[string]any{"n": 5}, 5},
		{map[string]any{}, 30},
		{map[string]any{"n": "bad"}, 30},
	}
	for _, c := range cases {
		if got := intOr(c.cfg, "n", 30); got != c.want {
			t.Fatalf("intOr(%v) = %d, want %d", c.cfg, got, c.want)
		}
	}
}
