// Package bot 框架运行时外壳: 配置加载、网关、HTTP 服务与生命周期控制。
package bot

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/KasumiYuku/Aurorix/lib/admin"
	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/assets"
	_ "github.com/KasumiYuku/Aurorix/lib/assets/providers"
	"github.com/KasumiYuku/Aurorix/lib/buttons"
	"github.com/KasumiYuku/Aurorix/lib/config"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/gateway"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/middleware"
	"github.com/KasumiYuku/Aurorix/lib/plugin"
	"github.com/KasumiYuku/Aurorix/lib/push"
	"github.com/KasumiYuku/Aurorix/lib/requests"
	"github.com/KasumiYuku/Aurorix/lib/schedule"
	"github.com/KasumiYuku/Aurorix/lib/state"
	"github.com/KasumiYuku/Aurorix/lib/stats"
	"github.com/KasumiYuku/Aurorix/lib/storage"
	"github.com/KasumiYuku/Aurorix/lib/structers"
	"github.com/KasumiYuku/Aurorix/lib/templates"
	"io"
	"net/http"
	"net/http/pprof"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	requestsClient *requests.Client = requests.Init(8)
	logger                          = logx.New("system")
)

type runOp int

const (
	opStop runOp = iota
	opRestart
)

// Run 启动框架并阻塞至退出。实例入口直接调用。
func Run() {
	appConfig := config.InitConfig()
	logx.Open(logx.Options{Ring: logx.DefaultRing, Level: appConfig.LogLevel})
	logger.Infof("运行时内存参数: %s", state.ApplyMemoryLimits(appConfig.MemoryLimitMB, appConfig.GCPercent))
	if err := ensureSingleInstance(appConfig.Port); err != nil {
		logger.Errorf("单实例检查失败: %v", err)
		os.Exit(1)
	}
	constant.SetPrefixChars(appConfig.Prefixes)
	buttons.SetCommandNormalizer(plugin.NormalizeCommandMsg)

	if err := storage.Open(appConfig.Database); err != nil {
		logger.Errorf("初始化 SQLite 存储失败: %v", err)
		os.Exit(1)
	}

	if err := plugin.LoadConfigurations(appConfig.PluginSettings); err != nil {
		logger.Errorf("部分插件配置未生效, 它们将跑在默认值上: %v", err)
	}
	accessConfigs := make(map[string]plugin.AccessConfig, len(appConfig.PluginAccess))
	for id, access := range appConfig.PluginAccess {
		commands := make(map[string]plugin.AccessRule, len(access.Commands))
		for path, rule := range access.Commands {
			commands[path] = plugin.AccessRule{Mode: rule.Mode, Users: rule.Users, Groups: rule.Groups}
		}
		accessConfigs[id] = plugin.AccessConfig{
			Default:  plugin.AccessRule{Mode: access.Default.Mode, Users: access.Default.Users, Groups: access.Default.Groups},
			Commands: commands,
			Disabled: access.Disabled,
		}
	}
	if err := plugin.LoadAccessConfigurations(accessConfigs); err != nil {
		logger.Errorf("部分插件访问控制未生效: %v", err)
	}
	client := api.Init(appConfig.AppId, appConfig.AppSecret, appConfig.ProxyAPI, requestsClient)

	assets.SetOfficialUpload(client.UploadOfficialImage)

	assetsManager := assets.NewManager(assets.NewClient(30))
	assetsManager.OnReload(client.SetAssets)
	if err := assetsManager.Load(); err != nil {
		logger.Warnf("读取 assets.json 失败，使用空图床配置: %v", err)
	}
	if host := assetsManager.Host(); host.Size() > 0 {
		logger.Infof("已启用图床聚合，provider 数量: %d", host.Size())
	}
	client.SetMessageOptions(appConfig.GlobalMarkdown, appConfig.RetryWhen, appConfig.UploadThreshold)

	push.Init(client)
	schedule.Start(client)
	state.Boot(appConfig.Protocol, int(appConfig.Port))

	if err := stats.Start(); err != nil {
		logger.Warnf("统计启动失败(数据不落库): %v", err)
	}

	profile := admin.NewProfileStore(client)
	if err := profile.Refresh(); err != nil {
		logger.Warnf("获取机器人档案失败: %v", err)
	} else if p := profile.Get(); p != nil {
		logger.Infof("机器人档案: %v (%v)", p.Username, p.ID)
	}

	opCh := make(chan runOp, 1)
	var controlOnce atomic.Bool
	queue := func(op runOp) {
		if controlOnce.CompareAndSwap(false, true) {
			opCh <- op
		}
	}

	mux := http.NewServeMux()
	var gw *gateway.Client
	admin.Register(mux, admin.Deps{
		Assets:  assetsManager,
		Client:  client,
		Profile: profile,
		Gateway: func() any {
			if gw != nil {
				return gw.Status()
			}
			return nil
		},
		Control: admin.Control{
			Restart:    func() { queue(opRestart) },
			Stop:       func() { queue(opStop) },
			Supervised: supervised(),
		},
	})

	plugin.UseControl(plugin.Control{
		Restartable: func() error {
			if !supervised() {
				return errors.New("未声明受外部守护(AURORIX_SUPERVISED=1), 重启会与守护进程各拉一个实例互杀")
			}
			return nil
		},
		Restart: func() { queue(opRestart) },
	})
	mux.HandleFunc("POST /push/{scope}/{openid}", push.HTTPHandle)

	if appConfig.GatewayURL != "" && appConfig.Protocol != "websocket" {
		logger.Warnf("已配置自定义网关地址, 但协议为 %s; 该项仅在 websocket 模式生效", appConfig.Protocol)
	}
	if appConfig.Protocol == "websocket" {
		src, err := gateway.SourceFor(client, appConfig.GatewayURL)
		if err != nil {
			logger.Errorf("获取网关地址失败: %v", err)
			os.Exit(1)
		}
		gw = gateway.New(client, src, gateway.Intents(appConfig.Intents), 0)
		go gw.Start()
	} else {
		mux.Handle("/webhook", middleware.VerifySignature(appConfig.AppSecret)(webhookHandler(client, appConfig)))
	}

	if appConfig.PProfAddr != "" {
		pprofMux := http.NewServeMux()
		pprofMux.HandleFunc("/debug/pprof/", pprof.Index)
		pprofMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		pprofMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		pprofMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		pprofMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		go func() {
			logger.Infof("pprof 已开启: http://%s/debug/pprof/ (仅本机可达, 用 SSH 隧道访问)", appConfig.PProfAddr)
			if err := http.ListenAndServe(appConfig.PProfAddr, pprofMux); err != nil {
				logger.Warnf("pprof 监听失败: %v", err)
			}
		}()
	}
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", appConfig.Port),
		Handler:           recoverHTTP(mux),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	logger.Infof("管理台: http://127.0.0.1:%d/admin", appConfig.Port)
	logger.Infof("注册了%v个Markdown模板, %v个HTML模板", templates.GetMarkdownTemplateCount(), templates.GetHTMLTemplateCount())
	logger.Infof("注册了%v个指令", plugin.GetCommandCount())
	logger.Infof("注册了%v个定时任务", schedule.GetJobCount())
	if gw != nil {
		logger.Infof("WebSocket 网关已启动（intents=%d）", len(appConfig.Intents))
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Errorf("HTTP 服务异常退出: %v", err)
			queue(opStop)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	op := opStop
	select {
	case <-ctx.Done():
		logger.Infof("收到退出信号")
	case op = <-opCh:
	}
	shutdown(srv, gw, op == opRestart)
}

func webhookHandler(client *api.BotAPI, appConfig config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return
		}
		var payload structers.Payload
		if err := json.Unmarshal(body, &payload); err != nil {
			return
		}
		var envelope struct {
			D json.RawMessage `json:"d"`
		}
		if json.Unmarshal(body, &envelope) == nil {
			payload.RawEvent = envelope.D
		}
		if payload.Op == 13 {
			_, privateKey := middleware.DeriveEd25519Key(appConfig.AppSecret)

			var msg bytes.Buffer
			msg.WriteString(payload.Data.EventTs.String())
			msg.WriteString(payload.Data.PlainToken)

			signature := hex.EncodeToString(ed25519.Sign(privateKey, msg.Bytes()))
			writeJSON(w, http.StatusOK, map[string]string{
				"plain_token": payload.Data.PlainToken,
				"signature":   signature,
			})
			return
		}

		if payload.T == "" {
			return
		}
		payload.EventType = constant.EventType(payload.T)
		middleware.ProcessAsync(payload, client)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func recoverHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Errorf("HTTP handler panic: %v", rec)
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func shutdown(srv *http.Server, gw *gateway.Client, restart bool) {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Warnf("HTTP 关闭超时: %v", err)
	}
	if gw != nil {
		gw.Stop()
	}
	schedule.Stop()
	stats.Stop()
	if err := storage.Close(); err != nil {
		logger.Warnf("SQLite 关闭失败: %v", err)
	}
	if restart {
		spawnSelf()
	}
	logger.Infof("进程退出")
}

func supervised() bool { return os.Getenv("AURORIX_SUPERVISED") != "" }

func spawnSelf() {
	if supervised() {
		logger.Infof("外部守护接管重启, 直接退出")
		return
	}
	exe, err := os.Executable()
	if err != nil {
		logger.Errorf("获取自身路径失败: %v", err)
		return
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = os.Environ()
	if dir, err := os.Getwd(); err == nil {
		cmd.Dir = dir
	}
	if err := cmd.Start(); err != nil {
		logger.Errorf("自重启失败: %v", err)
		return
	}
	logger.Infof("已拉起新进程 PID=%d", cmd.Process.Pid)
}

func ensureSingleInstance(port uint16) error {
	pid := portOwnerPID(port)
	if pid == 0 {
		return nil
	}
	if !isSelfBinary(pid) {
		logger.Warnf("端口 %d 被非本程序进程(pid=%d)占用, 不干预", port, pid)
		return nil
	}
	logger.Infof("检测到旧实例(pid=%d)占用端口 %d, 终止中", pid, port)
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		return fmt.Errorf("终止旧实例失败: %w", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	logger.Warnf("旧实例(pid=%d)未在 5 秒内退出, 强制终止", pid)
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		return fmt.Errorf("强制终止旧实例失败: %w", err)
	}
	return nil
}

func portOwnerPID(port uint16) int {
	hexPort := strings.ToUpper(fmt.Sprintf("%04X", port))
	want := make(map[string]bool)
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n")[1:] {
			f := strings.Fields(line)
			if len(f) < 10 || f[3] != "0A" {
				continue
			}
			local := f[1]
			sep := strings.LastIndex(local, ":")
			if sep < 0 || strings.ToUpper(local[sep+1:]) != hexPort {
				continue
			}
			want[f[9]] = true
		}
	}
	if len(want) == 0 {
		return 0
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if !isDigits(e.Name()) {
			continue
		}
		fds, err := os.ReadDir("/proc/" + e.Name() + "/fd")
		if err != nil {
			continue
		}
		for _, fd := range fds {
			link, err := os.Readlink("/proc/" + e.Name() + "/fd/" + fd.Name())
			if err != nil || len(link) < 9 || link[:8] != "socket:[" {
				continue
			}
			if want[link[8:len(link)-1]] {
				pid, _ := strconv.Atoi(e.Name())
				return pid
			}
		}
	}
	return 0
}

func isSelfBinary(pid int) bool {
	self, err := os.Executable()
	if err != nil {
		return false
	}
	name := filepath.Base(self)
	if comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid)); err == nil {
		if strings.TrimSpace(string(comm)) == name {
			return true
		}
	}
	if exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil {
		return filepath.Base(exe) == name
	}
	return false
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
