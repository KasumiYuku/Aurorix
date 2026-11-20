package contract

type CanMarshal interface {
	Marshal() ([]byte, error)
}

// PreSend 发送前预处理接口，某些消息需要在序列化前做最后处理。
type PreSend interface {
	Prepare()
}
