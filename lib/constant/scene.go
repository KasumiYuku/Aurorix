package constant

// Scene 会话场景; 同时是平台的 scope、存储的命名空间前缀与统计的分组维度。
type Scene string

const (
	SceneGroup   Scene = "group"
	SceneC2C     Scene = "c2c"
	SceneChannel Scene = "channel"
	SceneDirect  Scene = "dm"
)

// SceneLabels 场景中文名, 管理台、日志与提示语共用一份口径。
var SceneLabels = map[Scene]string{
	SceneGroup:   "群聊",
	SceneC2C:     "私聊",
	SceneChannel: "频道",
	SceneDirect:  "频道私信",
}

func (s Scene) Valid() bool {
	_, ok := SceneLabels[s]
	return ok
}

// Guild 报告这是不是频道域: 频道域用频道自己的 ID, 没有 openid, 也没有 v2 富媒体上传。
func (s Scene) Guild() bool {
	return s == SceneChannel || s == SceneDirect
}

func (s Scene) Label() string {
	if label, ok := SceneLabels[s]; ok {
		return label
	}
	return string(s)
}

// Target 一次投递的会话目标; ID 是场景内的会话标识(群/用户 openid、子频道 id、私信时的频道 id)。
type Target struct {
	Scene Scene
	ID    string
	User  string
	// MentionID 这个场景认的艾特 ID。频道里 User 是 union_openid, 而频道艾特只认 author.id 那种纯数字, 拿 union_openid 拼出来的 <@!…> 平台不渲染; 两者不是一回事, 所以分开存。
	MentionID string
}

// Mention 这个场景认的艾特 ID: 频道用 MentionID(纯数字), 其余退回 User。
func (t Target) Mention() string {
	if t.MentionID != "" {
		return t.MentionID
	}
	return t.User
}

func (t Target) Empty() bool {
	return t.Scene == "" || t.ID == ""
}

// String 给日志与错误信息用。
func (t Target) String() string {
	return string(t.Scene) + "/" + t.ID
}
