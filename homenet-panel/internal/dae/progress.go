package dae

import (
	"fmt"
	"os"
	"strings"
)

// dae 把重载/挂起结果写入 /var/run/dae.progress：
// 首字节是 ASCII 码点（'2'=ReloadDone, '3'=ReloadError），换行后是消息文本。
const (
	ProgressReloadDone  byte = '2'
	ProgressReloadError byte = '3'
	ProgressSuspendDone byte = '4'
)

type Progress struct {
	Code    byte
	Raw     string
	Message string
}

func ReadProgress(path string) (Progress, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Progress{}, err
	}
	if len(b) == 0 {
		return Progress{}, fmt.Errorf("empty progress file: %s", path)
	}
	p := Progress{Code: b[0], Raw: string(b)}
	rest := b[1:]
	p.Message = strings.TrimSpace(strings.TrimLeft(string(rest), "\r\n"))
	return p, nil
}

func (p Progress) IsDone() bool  { return p.Code == ProgressReloadDone }
func (p Progress) IsError() bool { return p.Code == ProgressReloadError }

// Describe 返回人类可读的状态名。
func (p Progress) Describe() string {
	switch p.Code {
	case ProgressReloadDone:
		return "ReloadDone"
	case ProgressReloadError:
		return "ReloadError"
	case ProgressSuspendDone:
		return "SuspendDone"
	default:
		return fmt.Sprintf("Unknown(%d)", p.Code)
	}
}
