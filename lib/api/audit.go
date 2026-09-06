package api

// 消息审核等待: 发送遇 304023 时挂起, 由 MESSAGE_AUDIT_PASS/REJECT 事件唤醒。

import (
	"fmt"
	"time"
)

const auditTimeout = 30 * time.Second

// waitAudit 注册审计等待并阻塞直到 audit 事件或超时。
func waitAudit(auditID string) error {
	ch := make(chan auditResult, 1)
	auditMu.Lock()
	auditWaiter[auditID] = ch
	auditMu.Unlock()
	defer func() {
		// 无论结果还是超时都移除等待项, 防止 map 滞留
		auditMu.Lock()
		delete(auditWaiter, auditID)
		auditMu.Unlock()
	}()

	select {
	case r := <-ch:
		if !r.approved {
			return fmt.Errorf("消息审核未通过")
		}
		return nil
	case <-time.After(auditTimeout):
		return fmt.Errorf("消息审核等待超时")
	}
}
