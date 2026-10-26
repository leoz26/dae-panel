package geodata

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct{ out map[string]string }

func (f fakeRunner) Run(name string, args ...string) (string, error) {
	key := name + " " + strings.Join(args, " ")
	if v, ok := f.out[key]; ok {
		return v, nil
	}
	return "", os.ErrNotExist
}

func TestDetectVersionsFileManaged(t *testing.T) {
	dir := t.TempDir()
	data := make([]byte, minSize+10)
	if err := os.WriteFile(filepath.Join(dir, GeoIPName), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, GeoSiteName), data, 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/geoip.dat"):
			w.Header().Set("Content-Length", "1234")
			w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		case strings.HasSuffix(r.URL.Path, "/geosite.dat"):
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		default:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "202610242205", "published_at": "2026-10-24T22:06:35Z"})
		}
	}))
	defer srv.Close()
	rep := DetectVersions(VersionOptions{
		Dir:    dir,
		APIURL: srv.URL + "/releases/latest",
		URLs:   map[string]string{"geoip": srv.URL + "/geoip.dat", "geosite": srv.URL + "/geosite.dat"},
		Runner: fakeRunner{},
	})
	if rep.RemoteTag != "202610242205" {
		t.Fatalf("远端 tag = %q", rep.RemoteTag)
	}
	if len(rep.Files) != 2 {
		t.Fatalf("文件数 = %d", len(rep.Files))
	}
	var geoIP, geoSite FileVersion
	for _, f := range rep.Files {
		if f.Name == GeoIPName {
			geoIP = f
		} else {
			geoSite = f
		}
	}
	if geoIP.UpToDate {
		t.Fatalf("大小不同应判为有更新：%+v", geoIP)
	}
	if !strings.Contains(geoIP.Verdict, "大小不同") {
		t.Fatalf("verdict = %q", geoIP.Verdict)
	}
	if !geoSite.UpToDate {
		t.Fatalf("大小相同应判为一致：%+v", geoSite)
	}
}

func TestDetectVersionsAptManaged(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), GeoSiteName)
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, GeoSiteName)); err != nil {
		t.Fatal(err)
	}
	runner := fakeRunner{out: map[string]string{
		"dpkg-query -W -f=${Version} v2ray-rules-dat": "202610222206.0.0",
		"apt-cache policy v2ray-rules-dat":            "v2ray-rules-dat:\n  Installed: 202610222206.0.0\n  Candidate: 202610242205.0.0\n",
	}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".dat") {
			w.Header().Set("Content-Length", "1")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "202610242205"})
	}))
	defer srv.Close()
	rep := DetectVersions(VersionOptions{
		Dir:    dir,
		APIURL: srv.URL + "/api",
		URLs:   map[string]string{"geosite": srv.URL + "/geosite.dat"},
		Runner: runner,
	})
	var fv FileVersion
	for _, f := range rep.Files {
		if f.Name == GeoSiteName {
			fv = f
		}
	}
	if !strings.Contains(fv.Managed, "apt") {
		t.Fatalf("managed = %q", fv.Managed)
	}
	if fv.PkgInstalled != "202610222206.0.0" || fv.PkgCandidate != "202610242205.0.0" {
		t.Fatalf("apt 版本读取失败：%+v", fv)
	}
	if fv.LocalTag != "202610222206" {
		t.Fatalf("localTag = %q", fv.LocalTag)
	}
	if fv.UpToDate {
		t.Fatal("apt 有候选新版本时应判为可更新")
	}
	if !strings.Contains(rep.Hint, "apt install --only-upgrade") {
		t.Fatalf("hint = %q", rep.Hint)
	}
}

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "geo-version.json")
	st := State{Tag: "202610242205", FetchedAt: time.Now(), Files: map[string]StateFile{"geosite": {Tag: "202610242205", Bytes: 3, SHA256: "abc"}}}
	if err := SaveState(path, st); err != nil {
		t.Fatal(err)
	}
	got := LoadState(path)
	if got.Tag != st.Tag || got.Files["geosite"].SHA256 != "abc" {
		t.Fatalf("round trip 失败：%+v", got)
	}
	if LoadState(filepath.Join(dir, "missing.json")).Tag != "" {
		t.Fatal("缺失文件应返回空 State")
	}
}

func TestUpdateWritesState(t *testing.T) {
	payload := make([]byte, minSize+100)
	copy(payload, []byte("\x00cn"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".dat") {
			_, _ = w.Write(payload)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "202610242205"})
	}))
	defer srv.Close()
	dir := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "geo-version.json")
	res, err := Update(UpdateOptions{
		Dir:       dir,
		BackupDir: filepath.Join(t.TempDir(), "backups"),
		Targets:   []string{"geoip"},
		URLs:      map[string]string{"geoip": srv.URL + "/geoip.dat"},
		StatePath: statePath,
	})
	if err != nil || len(res.Errors) != 0 {
		t.Fatalf("update 失败: %v %v", err, res.Errors)
	}
	st := LoadState(statePath)
	if st.Files["geoip"].Bytes != int64(len(payload)) {
		t.Fatalf("state 未记录：%+v", st)
	}
	if st.Files["geoip"].SHA256 == "" {
		t.Fatal("state 缺 sha256")
	}
}

func TestAptManagedSkipsHeadProbe(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), GeoIPName)
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, GeoIPName)); err != nil {
		t.Fatal(err)
	}
	var headHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".dat") {
			headHits++
			w.Header().Set("Content-Length", "1")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "202610242205"})
	}))
	defer srv.Close()
	rep := DetectVersions(VersionOptions{
		Dir:    dir,
		APIURL: srv.URL + "/api",
		URLs:   map[string]string{"geoip": srv.URL + "/geoip.dat"},
		Runner: fakeRunner{out: map[string]string{"dpkg-query -W -f=${Version} v2ray-rules-dat": "202610222206.0.0"}},
	})
	if headHits != 0 {
		t.Fatalf("apt 托管时不应探测 HEAD，实际 %d 次", headHits)
	}
	if rep.RemoteTag != "202610242205" {
		t.Fatalf("remoteTag = %q", rep.RemoteTag)
	}
}

func TestUsesReleaseAssetSizesInsteadOfHead(t *testing.T) {
	dir := t.TempDir()
	geoip := make([]byte, 100)
	geosite := make([]byte, 200)
	if err := os.WriteFile(filepath.Join(dir, GeoIPName), geoip, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, GeoSiteName), geosite, 0o644); err != nil {
		t.Fatal(err)
	}
	headHits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".dat") {
			headHits++
			w.Header().Set("Content-Length", "100")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name":     "202610242205",
			"published_at": "2026-10-24T22:06:35Z",
			"assets": []map[string]any{
				{"name": GeoIPName, "size": 100},
				{"name": GeoSiteName, "size": 200},
			},
		})
	}))
	defer srv.Close()
	rep := DetectVersions(VersionOptions{Dir: dir, APIURL: srv.URL + "/api", Runner: fakeRunner{}})
	if headHits != 0 {
		t.Fatalf("默认源有资产大小时不应发 HEAD，实际 %d 次", headHits)
	}
	for _, f := range rep.Files {
		if f.RemoteBytes == 0 {
			t.Fatalf("%s 未从 release 资产取到大小：%+v", f.Name, f)
		}
		if !f.UpToDate {
			t.Fatalf("%s 大小一致应判为一致：%+v", f.Name, f)
		}
	}
}
