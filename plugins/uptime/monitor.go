package uptime

import (
	"embed"
	"fmt"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/context"
	"github.com/KasumiYuku/Aurorix/lib/plugin"
	"github.com/KasumiYuku/Aurorix/lib/templates"
	"strings"
)

//go:embed templates/markdown/*.md
var templateFS embed.FS

func init() {
	commands := make([]*plugin.Command, 0)
	commands = append(commands, &plugin.Command{
		Prefix:   "uptime",
		Role:     constant.RoleAdmin,
		Describe: "服务状态帮助",
		Handle:   helpText,
	})
	commands = append(commands, &plugin.Command{
		Prefix:   "openai",
		Role:     constant.RoleMember,
		Describe: "OpenAI 服务状态",
		Handle:   openAIStatus,
	})

	self := &plugin.Plugin{
		Id:         "uptime",
		Commands:   commands,
		TemplateFS: templateFS,
	}
	plugin.Register(self)
}

func helpText(ctx *context.MessageContext) error {
	md, err := ctx.MarkdownTemplate("UptimeHelp", &templates.Args{})
	if err != nil {
		return err
	}
	return md.Send()
}

func openAIStatus(ctx *context.MessageContext) error {
	type Component struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	type AffectedComponent struct {
		ComponentID       string `json:"component_id"`
		OperationalStatus string `json:"operational_status"`
	}
	type Incident struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Impact string `json:"impact"`
	}

	var data struct {
		Summary struct {
			Components         []Component         `json:"components"`
			AffectedComponents []AffectedComponent `json:"affected_components"`
			OngoingIncidents   []Incident          `json:"ongoing_incidents"`
		} `json:"summary"`
	}

	err := ctx.Request.Get("https://status.openai.com/proxy/status.openai.com", &data, nil)
	if err != nil {
		return err
	}

	summary := data.Summary
	// 没有任何故障事件且没有受影响组件
	if len(summary.AffectedComponents) == 0 && len(summary.OngoingIncidents) == 0 {
		return ctx.Text("✅ OpenAI 当前所有服务均运行正常").Send()
	}

	// 建立 ID -> Name 映射表
	nameMap := make(map[string]string, len(summary.Components))
	for _, c := range summary.Components {
		nameMap[c.ID] = c.Name
	}

	// 状态枚举中文映射
	statusText := func(status string) string {
		switch status {
		case "degraded_performance":
			return "性能下降 ⚠️"
		case "partial_outage":
			return "部分中断 ❌"
		case "major_outage":
			return "严重宕机 🚨"
		case "under_maintenance":
			return "正在维护 🛠️"
		default:
			return status
		}
	}

	var sb strings.Builder
	sb.WriteString("⚠️ **OpenAI 服务异常提醒**\n")

	// 1. 正在发生的故障事件
	if len(summary.OngoingIncidents) > 0 {
		sb.WriteString("\n📌 **故障通报：**")
		for _, inc := range summary.OngoingIncidents {
			sb.WriteString(fmt.Sprintf("\n- %s (%s)", inc.Name, inc.Status))
		}
	}

	// 2. 受影响的具体组件
	if len(summary.AffectedComponents) > 0 {
		sb.WriteString("\n\n📉 **受影响服务：**")
		for _, v := range summary.AffectedComponents {
			name := nameMap[v.ComponentID]
			if name == "" {
				name = v.ComponentID
			}
			sb.WriteString(fmt.Sprintf("\n- **%s**: %s", name, statusText(v.OperationalStatus)))
		}
	}

	return ctx.Markdown(sb.String()).Send()
}
