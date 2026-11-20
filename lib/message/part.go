package message

// Part 消息部件标记：文本/艾特/markdown/图片/媒体都实现此接口。
type Part interface{ part() }
