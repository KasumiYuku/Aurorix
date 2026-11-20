package api

import (
	"fmt"
	"time"
)

const auditTimeout = 30 * time.Second

var auditWait = auditTimeout

func waitAudit(auditID string) error {
	ch := make(chan auditResult, 1)
	auditMu.Lock()
	auditWaiter[auditID] = ch
	auditMu.Unlock()
	defer func() {
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
	case <-time.After(auditWait):
		return fmt.Errorf("消息审核等待超时")
	}
}
