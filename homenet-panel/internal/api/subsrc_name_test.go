package api

import "testing"

func TestValidSubName(t *testing.T) {
	ok := []string{"summer", "dual", "CF", "my-sub_1.sub"}
	for _, n := range ok {
		if !validSubName(n) {
			t.Fatalf("应接受 %q", n)
		}
	}
	bad := []string{"", "../etc/passwd", "a/b", "有中文", "a b", "x\\y"}
	for _, n := range bad {
		if validSubName(n) {
			t.Fatalf("应拒绝 %q", n)
		}
	}
}
