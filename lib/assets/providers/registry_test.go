package providers

import (
	"testing"

	"github.com/KasumiYuku/Aurorix/lib/assets"
)

func TestAllRegistered(t *testing.T) {
	want := map[string]bool{
		"chatglm": true, "chinaunicom": true,
		"bilibili": true, "mihoyo": true, "sgame": true, "qqgame": true,
		"announce": true, "qzone": true, "homework": true, "qqstream": true,
		"kurobbs": true, "cnb": true,
	}
	names := assets.Names()
	for _, name := range names {
		delete(want, name)
	}
	if len(want) > 0 {
		missing := make([]string, 0, len(want))
		for n := range want {
			missing = append(missing, n)
		}
		t.Errorf("未注册的 provider: %v", missing)
	}
	if len(names) != 13 {
		t.Errorf("注册 provider 数 = %d, want 13(12 个第三方 + 官方图床 qqbot)", len(names))
	}
}

func boolPtr(b bool) *bool { return &b }

func TestNewHostEnabledFilter(t *testing.T) {
	cl := assets.NewClient(5)
	cfg := assets.HostConfig{
		Providers: []assets.ProviderItem{
			{Name: "smms", Enabled: boolPtr(false), Config: map[string]any{"token": "t"}},
			{Name: "chatglm"},
		},
	}
	host := assets.NewHost(cfg, cl)
	if host.Size() != 2 {
		t.Fatalf("enabled provider count = %d, want 2", host.Size())
	}
}

func TestNewHostSkipInvalidConfig(t *testing.T) {
	cl := assets.NewClient(5)
	cfg := assets.HostConfig{
		Providers: []assets.ProviderItem{{Name: "smms"}},
	}
	host := assets.NewHost(cfg, cl)
	if host.Size() != 1 {
		t.Fatalf("skipped provider count = %d, want 1", host.Size())
	}
}
