// Package bot 框架运行时外壳: 配置加载、网关、HTTP 服务与生命周期控制。
// 实例工程只需 import 本包并调用 Run()。
package bot

import (
	"Plrx/lib/admin"
	"Plrx/lib/api"
	"Plrx/lib/assets"
	_ "Plrx/lib/assets/providers"
	"Plrx/lib/buttons"
	"Plrx/lib/config"
	"Plrx/lib/constant"
	"Plrx/lib/gateway"
	"Plrx/lib/logx"
	"Plrx/lib/middleware"
	"Plrx/lib/plugin"
	"Plrx/lib/push"
	"Plrx/lib/requests"
	"Plrx/lib/schedule"
	"Plrx/lib/state"
	"Plrx/lib/storage"
	"Plrx/lib/structers"
	"Plrx/lib/templates"
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	requestsClient *requests.Client = requests.Init(8)
	logger                          = logx.New("system")
)

// runOp 运行控制指令。
type runOp int

const (
	opStop runOp = iota
	opRestart
)

// Run 启动框架并阻塞至退出。实例入口直接调用。
func Run() {
	appConfig := config.InitConfig()
	logx.Open(logx.Options{Ring: logx.DefaultRing, Level: appConfig.LogLevel})
	if err := ensureSingleInstance(appConfig.Port); err != nil {
		logger.Errorf("单实例检查失败: %v", err)
		os.Exit(1)
	}
	constant.SetPrefixChars(appConfig.Prefixes)
	buttons.SetCommandNormalizer(plugin.NormalizeCommandMsg)

	// 初始化相关配置
	if err := plugin.LoadConfigurations(appConfig.PluginSettings); err != nil {
		logger.Errorf("加载插件配置失败: %v", err)
		os.Exit(1)
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
		logger.Errorf("加载插件访问控制失败: %v", err)
		os.Exit(1)
	}
	if err := storage.Open(appConfig.Database); err != nil {
		logger.Errorf("初始化 SQLite 存储失败: %v", err)
		os.Exit(1)
	}
	client := api.Init(appConfig.AppId, appConfig.AppSecret, appConfig.ProxyAPI, requestsClient)

	// 图床聚合器：配置独立存放于 assets.json+
	assetsManager := assets.NewManager(assets.NewClient(30))
	assetsManager.OnReload(client.SetAssets)
	if err := assetsManager.Load(); err != nil {
		logger.Warnf("读取 assets.json 失败，使用空图床配置: %v", err)
	}
	if host := assetsManager.Host(); host.Size() > 0 {
		logger.Infof("已启用图床聚合，provider 数量: %d", host.Size())
	}
	client.SetMessageOptions(appConfig.GlobalMarkdown, appConfig.MarkdownVerifyImage, appConfig.RetryWhen, appConfig.UploadThreshold)

	push.Init(client)
	schedule.Start(client)
	state.Boot(appConfig.Protocol, int(appConfig.Port))

	// 运行控制通道, 首条指令生效, 防重复触发
	opCh := make(chan runOp, 1)
	var controlOnce atomic.Bool
	queue := func(op runOp) {
		if controlOnce.CompareAndSwap(false, true) {
			opCh <- op
		}
	}

	engine := gin.New()
	engine.Use(gin.Recovery())
	var gw *gateway.Client // websocket 模式在下方赋值, 探针闭包按请求时读取
	admin.Register(engine, admin.Deps{
		Assets: assetsManager,
		Client: client,
		Gateway: func() any {
			if gw != nil {
				return gw.Status()
			}
			return nil
		},
		Control: admin.Control{
			Restart: func() { queue(opRestart) },
			Stop:    func() { queue(opStop) },
		},
	})
	engine.POST("/push/:scope/:openid", push.HTTPHandle)

	if appConfig.Protocol == "websocket" {
		gatewayURL, err := client.GatewayURL()
		if err != nil {
			logger.Errorf("获取网关地址失败: %v", err)
			os.Exit(1)
		}
		intents := gateway.Intents(appConfig.Intents)
		gw = gateway.New(client, gatewayURL, intents, [2]int{0, 1})
		go gw.Start()
	} else {
		// 仅 webhook 需要QQ签名校验
		webhook := engine.Group("/")
		webhook.Use(middleware.VerifySignature(appConfig.AppSecret))
		webhook.POST("/webhook", webhookHandler(client, appConfig))
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", appConfig.Port),
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Infof("管理台: http://127.0.0.1:%d/admin", appConfig.Port)
	logger.Infof("注册了%v个Markdown模板", templates.GetMarkdownTemplateCount())
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

// webhookHandler QQ 回调入口。
func webhookHandler(client *api.BotAPI, appConfig config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Status(http.StatusOK)
		body, err := c.GetRawData()
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
		// Op = 13, 签名验证
		if payload.Op == 13 {
			_, privateKey := middleware.DeriveEd25519Key(appConfig.AppSecret)

			var msg bytes.Buffer
			msg.WriteString(payload.Data.EventTs)
			msg.WriteString(payload.Data.PlainToken)

			signature := hex.EncodeToString(ed25519.Sign(privateKey, msg.Bytes()))
			c.JSON(http.StatusOK, gin.H{
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

// shutdown 优雅退出; restart 为真时重新拉起自身。
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
	if err := storage.Close(); err != nil {
		logger.Warnf("SQLite 关闭失败: %v", err)
	}
	if restart {
		spawnSelf()
	}
	logger.Infof("进程退出")
}

// spawnSelf 以相同参数重新拉起自身。外部守护管理时直接退出交由守护拉起。
func spawnSelf() {
	if os.Getenv("POLARIX_SUPERVISED") != "" {
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

// ensureSingleInstance 终止占用指定端口的同名旧实例, 保证单实例启动。
// 占用者非本程序进程时仅告警, 交由端口冲突的自然失败处理, 避免误杀。
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

// portOwnerPID 反查监听端口的进程 pid: /proc/net/tcp 匹配 LISTEN inode, 再扫 /proc/<pid>/fd 定位。
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
			if len(f) < 10 || f[3] != "0A" { // 仅 LISTEN 状态
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

// isSelfBinary 判断 pid 是否为同名程序: 匹配进程名或可执行文件 basename。
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
