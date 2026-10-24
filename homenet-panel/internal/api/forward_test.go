package api

import "testing"

func TestForwardIfaceFromError(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Error: reload failed: publish staged reload candidate: bind LAN interface ens2: attach interface ens2: ipforward on ens2 is off: /proc/sys/net/ipv4/conf/ens2/forwarding", "ens2"},
		{"ipforward on br-lan is off", "br-lan"},
		{"Error: reload failed: something else", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := forwardIfaceFromError(c.in); got != c.want {
			t.Fatalf("forwardIfaceFromError(%q)=%q, want %q", c.in, got, c.want)
		}
	}
}
