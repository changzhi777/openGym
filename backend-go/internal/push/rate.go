// rate.go — 限频窗口构造器(安全对齐,上游 febb2b2b3;实现见 handlers.go rateWindow)。
package push

import "time"

// NewRateWindow 构造按账号的固定窗口限频器(max 次/win 时长)。
func NewRateWindow(max int, win time.Duration) *rateWindow {
	return newRateWindow(max, win)
}
