package diag

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	cases := map[string]string{
		" INFO ALIVE --tcp4-> NOT ALIVE:  dialer=【隧道】美国 P2S group=proxy":                                                               KindNotAlive,
		" WARN Marking dialer as unavailable due to persistent proxy IP failures dialer=【隧道】美国 R5 network=udp6(DNS),udp4 promotions=1": KindProxyIPFailure,
		" INFO Group re-selects dialer _new_dialer=A _old_dialer=B alive_dialers=6 group=hk":                                           KindReselect,
		" INFO Group selects dialer dialer=A group=hk network=udp4":                                                                    KindSelect,
		" WARN x error=bootstrap resolver returned no usable address for \"cdn-hw.sonlo365.com\" network=udp+4 outbound=hk":            KindBootstrap,
		" WARN DNS forward to upstream failed dialer=direct error=dial tcp <nil>->223.5.5.5:853: i/o timeout upstream":                 KindIOTimeout,
		" WARN DNS ingress fast path failed; sending SERVFAIL response dst=192.168.1.85:53":                                            KindServfail,
		" WARN no dialer in this group group=hk":                                                                                       KindNoDialer,
		" INFO something else entirely":                                                                                                "",
	}
	for line, want := range cases {
		if got := Classify(line); got != want {
			t.Fatalf("Classify(%q)=%q want %q", line, got, want)
		}
	}
}

const ts1 = "2026-10-28T12:52:11+08:00 "

func TestParseAggregates(t *testing.T) {
	lines := []string{
		ts1 + "Debian-te dae[87772]:  WARN DNS forward to upstream failed dialer=direct error=dial tcp <nil>->223.5.5.5:853: i/o timeout upstream",
		ts1 + "Debian-te dae[87772]:  WARN DNS forward to upstream failed dialer=direct error=dial tcp <nil>->223.5.5.5:853: i/o timeout upstream",
		"2026-10-28T12:52:35+08:00 Debian-te dae[87772]:  WARN x error=bootstrap resolver returned no usable address for \"cdn-hw.sonlo365.com\" outbound=hk",
		"2026-10-28T12:53:10+08:00 Debian-te dae[87772]:  WARN DNS ingress fast path failed; sending SERVFAIL response dst=192.168.1.85:53",
		"2026-10-28T12:53:58+08:00 Debian-te dae[87772]:  INFO ALIVE --udp4(DNS)-> NOT ALIVE:  dialer=【隧道】美国 P2S group=AI",
		"2026-10-28T12:53:59+08:00 Debian-te dae[87772]:  INFO ALIVE --udp4(DNS)-> NOT ALIVE:  dialer=【隧道】美国 P2S group=proxy",
		"2026-10-28T12:54:00+08:00 Debian-te dae[87772]:  WARN Marking dialer as unavailable due to persistent proxy IP failures dialer=【隧道】美国 R5 network=udp6(DNS),udp4 promotions=1",
		"2026-10-28T12:54:10+08:00 Debian-te dae[87772]:  INFO Group re-selects dialer _new_dialer=X _old_dialer=Y alive_dialers=6 group=hk latency_delta_ms=0",
		"2026-10-28T12:54:20+08:00 Debian-te dae[87772]:  INFO Group selects dialer dialer=X group=hk network=udp4",
		"2026-10-28T12:55:00+08:00 Debian-te dae[87772]:  INFO unrelated line",
	}
	rep := Parse(lines, 60, "test", time.Now())
	if rep.TotalLines != 10 || rep.Events != 9 {
		t.Fatalf("total/events = %d/%d", rep.TotalLines, rep.Events)
	}
	if rep.Counts[KindIOTimeout] != 2 || rep.Counts[KindBootstrap] != 1 || rep.Counts[KindServfail] != 1 {
		t.Fatalf("counts=%v", rep.Counts)
	}
	if rep.Counts[KindNotAlive] != 2 || rep.Counts[KindReselect] != 1 {
		t.Fatalf("counts=%v", rep.Counts)
	}
	if len(rep.Nodes) != 2 {
		t.Fatalf("nodes=%+v", rep.Nodes)
	}
	if rep.Nodes[0].Name != "【隧道】美国 P2S" || rep.Nodes[0].NotAlive != 2 {
		t.Fatalf("top node=%+v", rep.Nodes[0])
	}
	var hk, ai bool
	for _, g := range rep.Groups {
		if g.Name == "hk" && g.Reselects == 1 && g.Selects == 1 && g.AliveDialers == 6 {
			hk = true
		}
		if g.Name == "AI" && g.NotAlive == 1 {
			ai = true
		}
	}
	if !hk || !ai {
		t.Fatalf("groups=%+v", rep.Groups)
	}
	if len(rep.Series) != 3 {
		t.Fatalf("series=%+v", rep.Series)
	}
	joined := strings.Join(rep.Advice, " | ")
	for _, want := range []string{"bootstrap", "DoT 853", "误杀最多的节点"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("advice missing %q: %s", want, joined)
		}
	}
}

func TestParseWithoutTimestamps(t *testing.T) {
	lines := []string{
		"Wed Oct 28 12:52:11 2026 user.warn dae:  WARN sending SERVFAIL response dst=192.168.1.11:53",
		"Wed Oct 28 12:52:12 2026 user.info dae:  INFO Group selects dialer dialer=X group=hk network=udp4",
	}
	rep := Parse(lines, 30, "logread", time.Now())
	if rep.Counts[KindServfail] != 1 || rep.Counts[KindSelect] != 1 {
		t.Fatalf("counts=%v", rep.Counts)
	}
	if len(rep.Series) != 0 {
		t.Fatalf("series should be empty without ts: %+v", rep.Series)
	}
}

type fakeRunner struct {
	out  map[string]string
	errs map[string]error
}

func (f fakeRunner) Run(name string, args ...string) (string, error) {
	key := name
	if e, ok := f.errs[key]; ok && e != nil {
		return "", e
	}
	return f.out[key], nil
}

func TestFromCommandFallsBackToLogread(t *testing.T) {
	f := fakeRunner{
		out: map[string]string{
			"logread": "user.warn dae:  WARN sending SERVFAIL response dst=192.168.1.11:53\nuser.info dae:  INFO Group selects dialer dialer=X group=hk network=udp4\nunrelated line\n",
		},
		errs: map[string]error{"journalctl": errors.New("no journal")},
	}
	rep := FromCommand(f, "dae", 15, time.Now())
	if rep.Source != "logread" {
		t.Fatalf("source=%s", rep.Source)
	}
	if rep.Counts[KindServfail] != 1 || rep.Counts[KindSelect] != 1 {
		t.Fatalf("counts=%v", rep.Counts)
	}
	if rep.TotalLines != 2 {
		t.Fatalf("logread filter kept %d lines", rep.TotalLines)
	}
}

func TestAdviceFrequentReselect(t *testing.T) {
	lines := []string{}
	for i := 0; i < 12; i++ {
		lines = append(lines, "2026-10-28T12:54:10+08:00 dae[1]:  INFO Group re-selects dialer _new_dialer=X _old_dialer=Y alive_dialers=4 group=hk latency_delta_ms=0")
	}
	rep := Parse(lines, 60, "test", time.Now())
	joined := strings.Join(rep.Advice, " | ")
	if !strings.Contains(joined, "选点切换") || !strings.Contains(joined, "min_avg10") {
		t.Fatalf("advice=%s", joined)
	}
}

func TestFromCommandJournalError(t *testing.T) {
	f := fakeRunner{out: map[string]string{}, errs: map[string]error{"journalctl": errors.New("denied"), "logread": errors.New("denied")}}
	rep := FromCommand(f, "dae", 15, time.Now())
	if rep.Error == "" {
		t.Fatal("expected error")
	}
	if len(rep.Advice) == 0 || !strings.Contains(rep.Advice[0], "读取日志失败") {
		t.Fatalf("advice=%v", rep.Advice)
	}
}
