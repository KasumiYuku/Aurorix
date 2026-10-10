package assets

// OfficialUploadFunc 一次官方图床上传：收图片输入（含会话目标），返回公网直链。
type OfficialUploadFunc func(in ProviderInput) (ResolvedImage, error)

var officialUpload OfficialUploadFunc

// SetOfficialUpload 注入官方上传实现，幂等，最后一次生效；传 nil 表示停用。
func SetOfficialUpload(fn OfficialUploadFunc) { officialUpload = fn }

// OfficialUploadFn 返回当前实现，未注入时为 nil。
func OfficialUploadFn() OfficialUploadFunc { return officialUpload }
