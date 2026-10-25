package geodata

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeDat(t *testing.T, tags ...string) []byte {
	t.Helper()
	pad := strings.Repeat("A", 1<<20)
	return []byte(pad + "\x00" + strings.Join(tags, "\x00"))
}

func TestResolve(t *testing.T) {
	for _, in := range []string{"geoip", "GEOIP", " geoip.dat "} {
		if n, ok := Resolve(in); !ok || n != GeoIPName {
			t.Fatalf("Resolve(%q) = %q,%v", in, n, ok)
		}
	}
	if _, ok := Resolve("dns.dat"); ok {
		t.Fatal("未知目标不应被接受")
	}
}

func TestReadTagsFindsTagsCaseInsensitively(t *testing.T) {
	p := filepath.Join(t.TempDir(), GeoSiteName)
	if err := os.WriteFile(p, fakeDat(t, "CN", "APPLE-CN", "CATEGORY-AI-!CN"), 0o600); err != nil {
		t.Fatal(err)
	}
	tags, err := ReadTags(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"cn", "apple-cn", "category-ai-!cn"} {
		if !tags[want] {
			t.Fatalf("缺少 tag %q", want)
		}
	}
}

func TestUpdateReplacesAndBackups(t *testing.T) {
	dir := t.TempDir()
	backupRoot := filepath.Join(t.TempDir(), "backups")
	old := []byte("old-content")
	if err := os.WriteFile(filepath.Join(dir, GeoSiteName), old, 0o644); err != nil {
		t.Fatal(err)
	}
	newData := fakeDat(t, "CN", "YOUTUBE")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(newData)
	}))
	defer srv.Close()

	res, err := Update(UpdateOptions{
		Dir:       dir,
		BackupDir: backupRoot,
		Targets:   []string{"geosite"},
		URLs:      map[string]string{"geosite": srv.URL},
		Keep:      1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("更新报错: %v", res.Errors)
	}
	if len(res.Updated) != 1 || res.Updated[0].Tags < 2 {
		t.Fatalf("Updated=%+v", res.Updated)
	}
	got, _ := os.ReadFile(filepath.Join(dir, GeoSiteName))
	if string(got) != string(newData) {
		t.Fatal("文件未被替换")
	}
	backups := ListBackups(backupRoot)
	if len(backups) != 1 {
		t.Fatalf("备份数量 = %d，期望 1", len(backups))
	}
	if b, _ := os.ReadFile(filepath.Join(BackupDir(backupRoot), backups[0].Name)); string(b) != string(old) {
		t.Fatal("备份内容不是旧文件")
	}
}

func TestUpdateDryRunDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	newData := fakeDat(t, "CN")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(newData)
	}))
	defer srv.Close()
	res, err := Update(UpdateOptions{Dir: dir, BackupDir: t.TempDir(), Targets: []string{"geoip"}, URLs: map[string]string{"geoip": srv.URL}, DryRun: true})
	if err != nil || len(res.Updated) != 1 {
		t.Fatalf("dry-run 失败: %v %+v", err, res)
	}
	if _, err := os.Stat(filepath.Join(dir, GeoIPName)); !os.IsNotExist(err) {
		t.Fatal("dry-run 不应写文件")
	}
}

func TestUpdateRejectsSmallPayload(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>404</html>"))
	}))
	defer srv.Close()
	res, err := Update(UpdateOptions{Dir: dir, BackupDir: t.TempDir(), Targets: []string{"geoip"}, URLs: map[string]string{"geoip": srv.URL}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) != 1 || len(res.Updated) != 0 {
		t.Fatalf("应当拒绝小文件: %+v", res)
	}
}

func TestUpdateRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(t.TempDir(), "real-geosite.dat")
	if err := os.WriteFile(real, fakeDat(t, "CN"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, GeoSiteName)
	if err := os.Symlink(real, link); err != nil {
		t.Skip("当前环境不支持符号链接")
	}
	newData := fakeDat(t, "CN")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(newData)
	}))
	defer srv.Close()
	res, err := Update(UpdateOptions{Dir: dir, BackupDir: t.TempDir(), Targets: []string{"geosite"}, URLs: map[string]string{"geosite": srv.URL}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Updated) != 0 || len(res.Errors) != 1 {
		t.Fatalf("应当拒绝符号链接: %+v", res)
	}
	if li, _ := os.Lstat(link); li.Mode()&os.ModeSymlink == 0 {
		t.Fatal("符号链接被替换成了普通文件")
	}
	st := Inspect(dir, t.TempDir())
	if len(st.Files) != 2 || st.Files[1].Managed == "" {
		t.Fatalf("Inspect 应标出 symlink: %+v", st.Files)
	}
}
