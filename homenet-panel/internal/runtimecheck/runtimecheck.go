package runtimecheck

import (
	"os"
	"strings"
)

type Report struct {
	KernelRelease string
	BTF           bool
	Memlock       string
	UnprivBPF     string
}

func Run() Report {
	var r Report
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		r.KernelRelease = strings.TrimSpace(string(b))
	}
	if _, err := os.Stat("/sys/kernel/btf/vmlinux"); err == nil {
		r.BTF = true
	}
	if b, err := os.ReadFile("/proc/sys/kernel/unprivileged_bpf_disabled"); err == nil {
		r.UnprivBPF = strings.TrimSpace(string(b))
	}
	r.Memlock = readLimit("Max locked memory")
	return r
}

func readLimit(name string) string {
	b, err := os.ReadFile("/proc/self/limits")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, name) {
			f := strings.Fields(line)
			if len(f) >= 6 {
				// name(word) soft hard units
				return strings.Join(f[len(f)-3:], " ")
			}
		}
	}
	return ""
}
