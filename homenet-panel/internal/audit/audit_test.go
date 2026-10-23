package audit

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestAppendAndTail(t *testing.T) {
	p := filepath.Join(t.TempDir(), "audit.log")
	if err := Append(p, Entry{Actor: "admin", Action: "login", OK: true}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := Append(p, Entry{Actor: "admin", Action: "apply", OK: false, Detail: "x"}); err != nil {
		t.Fatalf("append2: %v", err)
	}
	items, err := Tail(p, 10)
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2, got %d", len(items))
	}
	if items[0].Action != "apply" {
		t.Errorf("最新应在前, got %q", items[0].Action)
	}
	if items[1].TS == "" {
		t.Errorf("TS 未填")
	}
	fi, _ := os.Stat(p)
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("权限应为 0600, got %o", fi.Mode().Perm())
	}
	if got, _ := Tail(p, 1); len(got) != 1 {
		t.Errorf("limit 生效失败: %d", len(got))
	}
}

func TestTailMissingFile(t *testing.T) {
	items, err := Tail(filepath.Join(t.TempDir(), "none.log"), 5)
	if err != nil || len(items) != 0 {
		t.Fatalf("缺失文件应返回空, err=%v n=%d", err, len(items))
	}
}

func TestAppendEscapesNewline(t *testing.T) {
	p := filepath.Join(t.TempDir(), "audit.log")
	if err := Append(p, Entry{Detail: "line1\nline2"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if strings.Count(string(b), "\n") != 1 {
		t.Errorf("记录应单行(JSON 转义换行): %q", b)
	}
}

// 轮转必须只保留配置的份数，且 ReadAll 能跨历史文件按时间正序读出。
func TestRotationKeepsN(t *testing.T) {
	defer SetRetention(DefaultMaxBytes, DefaultKeep)
	SetRetention(300, 2)
	p := filepath.Join(t.TempDir(), "audit.log")
	for i := 0; i < 40; i++ {
		if err := Append(p, Entry{TS: "2026-01-01T00:00:0" + string(rune('0'+i%10)) + "Z", Action: "act", Detail: strings.Repeat("x", 60)}); err != nil {
			t.Fatal(err)
		}
	}
	files, total := Files(p)
	if len(files) != 3 { // 当前 + 2 份历史
		t.Fatalf("应保留 3 个文件，实际 %d: %v", len(files), files)
	}
	if total > 3*300*3 {
		t.Fatalf("总大小异常: %d", total)
	}
	if _, err := os.Stat(p + ".3"); !os.IsNotExist(err) {
		t.Fatalf("超出保留份数的文件应被删除")
	}
	all, err := ReadAll(p)
	if err != nil || len(all) == 0 {
		t.Fatalf("ReadAll 失败: %v n=%d", err, len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].TS > all[i].TS {
			t.Fatalf("导出应时间正序: %q 在 %q 之前", all[i-1].TS, all[i].TS)
		}
	}
}

// CSV 导出要能处理逗号/引号/换行，并带 BOM（Excel 中文不乱码）。
func TestCSVExport(t *testing.T) {
	b, err := CSV([]Entry{{TS: "2026-01-01T00:00:00Z", Actor: "admin", Action: "login", OK: true, Detail: "a,b\"c\nd"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), "\ufeff") {
		t.Fatalf("缺少 UTF-8 BOM")
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(b), "\ufeff"))).ReadAll()
	if err != nil {
		t.Fatalf("CSV 解析失败: %v", err)
	}
	if len(rows) != 2 || rows[0][0] != "时间" {
		t.Fatalf("表头异常: %v", rows)
	}
	if rows[1][5] != "成功" || !strings.Contains(rows[1][6], "\"c") {
		t.Fatalf("字段异常: %v", rows[1])
	}
}

// Tail 应能跨历史文件取到最近记录。
func TestTailAcrossRotation(t *testing.T) {
	defer SetRetention(DefaultMaxBytes, DefaultKeep)
	SetRetention(200, 2)
	p := filepath.Join(t.TempDir(), "audit.log")
	for i := 0; i < 20; i++ {
		if err := Append(p, Entry{Action: "a" + strconv.Itoa(i), Detail: strings.Repeat("y", 40)}); err != nil {
			t.Fatal(err)
		}
	}
	items, err := Tail(p, 30)
	if err != nil {
		t.Fatal(err)
	}
	// 阈值 200B、保留 2 份：3 个文件各约 3 条，能跨轮转取到多条即说明逻辑生效
	if len(items) < 4 {
		t.Fatalf("跨轮转取记录不足: %d", len(items))
	}
	if items[0].Action != "a19" {
		t.Fatalf("最新记录应为 a19，实际 %q", items[0].Action)
	}
}
