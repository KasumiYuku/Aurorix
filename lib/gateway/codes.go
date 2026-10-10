package gateway

const (
	codeAuthFail   = 4004
	codeSessGone   = 4006
	codeInvalidSeq = 4007
	codeBotOffline = 4914
	codeBotBanned  = 4915
)

func canResume(code int) bool {
	switch code {
	case codeBotOffline, codeBotBanned:
		return false
	}
	return true
}

func needReidentify(code int) bool {
	switch code {
	case codeAuthFail, codeSessGone, codeInvalidSeq:
		return true
	}
	return false
}
