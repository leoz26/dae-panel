package mesh

import (
	"errors"
	"strings"
	"testing"
)

type fakeRunner struct {
	out map[string]string
	err map[string]error
}

func (f fakeRunner) Run(name string, args ...string) (string, error) {
	key := name
	if len(args) > 0 {
		key += " " + strings.Join(args, " ")
	}
	if e, ok := f.err[key]; ok {
		return f.out[key], e
	}
	if v, ok := f.out[key]; ok {
		return v, nil
	}
	return "", errors.New("not found")
}

const peerOut = `| ipv4          | hostname  | cost  | lat(ms) | loss | rx      | tx      | tunnel | NAT            | version        |
|---------------|-----------|-------|---------|------|---------|---------|--------|----------------|----------------|
| 10.144.0.2/24 | Debian-te | Local | -       | -    | -       | -       | -      | PortRestricted | 2.6.4-8428a89d |
| 10.144.0.1/24 | armbian   | p2p   | 11.89   | 0.0% | 1.21 kB | 1.59 kB | wss6   | PortRestricted | 2.6.4-8428a89d |
`

const routeOut = `| ipv4           | hostname | proxy_cidrs   | next_hop_id | next_hop_hostname | cost | path_latency | version        |
|----------------|----------|---------------|-------------|-------------------|------|--------------|----------------|
| 10.144.0.1/24  | armbian  | 192.168.5.0/24 | 10.144.0.1 | armbian           | 1    | 15           | 2.6.4-8428a89d |
`

func TestParseTableSkipsSeparators(t *testing.T) {
	rows := ParseTable(peerOut)
	if len(rows) != 3 {
		t.Fatalf("rows=%d want 3: %v", len(rows), rows)
	}
	if rows[0][0] != "ipv4" || rows[0][1] != "hostname" {
		t.Fatalf("header=%v", rows[0])
	}
}

func TestParsePeers(t *testing.T) {
	peers := ParsePeers(peerOut)
	if len(peers) != 2 {
		t.Fatalf("peers=%d", len(peers))
	}
	if peers[1].Hostname != "armbian" || peers[1].Tunnel != "wss6" || peers[1].Latency != "11.89" {
		t.Fatalf("peer=%+v", peers[1])
	}
	if peers[0].Cost != "Local" {
		t.Fatalf("peer0=%+v", peers[0])
	}
}

func TestParseRoutes(t *testing.T) {
	routes := ParseRoutes(routeOut)
	if len(routes) != 1 {
		t.Fatalf("routes=%d", len(routes))
	}
	if routes[0].ProxyCIDRs != "192.168.5.0/24" || routes[0].NextHopHost != "armbian" {
		t.Fatalf("route=%+v", routes[0])
	}
}

func baseRunner() fakeRunner {
	return fakeRunner{
		out: map[string]string{
			"test -f /etc/openwrt_release": "",
			"test -d /run/systemd/system":  "",
			"systemctl is-active easytier": "active\n",
			"systemctl restart easytier":   "",
			"systemctl start easytier":     "",
			"easytier-core --version":      "easytier-core 2.6.4-8428a89d\n",
			"easytier-cli peer":            peerOut,
			"easytier-cli route":           routeOut,
		},
		err: map[string]error{
			"test -f /etc/openwrt_release": errors.New("exit 1"),
		},
	}
}

func TestDetectRunning(t *testing.T) {
	st := Detect(Options{Runner: baseRunner()})
	if !st.Installed || !st.Running {
		t.Fatalf("status=%+v", st)
	}
	if st.ServiceManager != svcSystemd {
		t.Fatalf("mgr=%s", st.ServiceManager)
	}
	if len(st.Peers) != 2 || len(st.Routes) != 1 {
		t.Fatalf("peers=%d routes=%d", len(st.Peers), len(st.Routes))
	}
	if st.Note != "" {
		t.Fatalf("note=%s", st.Note)
	}
}

func TestDetectNotInstalled(t *testing.T) {
	r := fakeRunner{out: map[string]string{}, err: map[string]error{
		"test -f /etc/openwrt_release": errors.New("exit 1"),
		"test -d /run/systemd/system":  nil,
		"easytier-core --version":      errors.New("exit 127"),
	}}
	st := Detect(Options{Runner: r})
	if st.Installed {
		t.Fatalf("should not be installed")
	}
	if st.ConfigHint == "" {
		t.Fatalf("want hint")
	}
}

func TestDetectStopped(t *testing.T) {
	r := baseRunner()
	r.out["systemctl is-active easytier"] = "inactive\n"
	r.err["systemctl is-active easytier"] = errors.New("exit 3")
	st := Detect(Options{Runner: r})
	if !st.Installed || st.Running {
		t.Fatalf("status=%+v", st)
	}
	if st.ConfigHint == "" {
		t.Fatalf("want hint")
	}
}

func TestDetectNoteWhenAlone(t *testing.T) {
	r := baseRunner()
	r.out["easytier-cli peer"] = `| ipv4          | hostname  | cost  | lat(ms) | loss | rx | tx | tunnel | NAT | version |
|---------------|-----------|-------|---------|------|----|----|--------|-----|---------|
| 10.144.0.2/24 | Debian-te | Local | -       | -    | -  | -  | -      | -   | 2.6.4   |
`
	st := Detect(Options{Runner: r})
	if st.Note == "" {
		t.Fatalf("want note when alone")
	}
}

func TestControlSystemd(t *testing.T) {
	r := baseRunner()
	msg, err := Control(Options{Runner: r}, "restart")
	if err != nil || !strings.Contains(msg, "systemctl restart easytier") {
		t.Fatalf("msg=%q err=%v", msg, err)
	}
}

func TestControlProcd(t *testing.T) {
	r := baseRunner()
	r.err = map[string]error{}
	if _, err := Control(Options{Runner: r}, "start"); err == nil {
		if mgr := detectServiceManager(Options{Runner: r}); mgr != svcProcd {
			t.Fatalf("mgr=%s", mgr)
		}
	}
}

func TestControlUnknownManager(t *testing.T) {
	r := fakeRunner{err: map[string]error{"test -f /etc/openwrt_release": errors.New("x"), "test -d /run/systemd/system": errors.New("x")}}
	if _, err := Control(Options{Runner: r}, "start"); err == nil {
		t.Fatalf("want error for unknown manager")
	}
}
