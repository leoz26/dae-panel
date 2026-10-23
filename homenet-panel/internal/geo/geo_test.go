package geo

import "testing"

func encVarint(x uint64) []byte {
	var out []byte
	for x >= 0x80 {
		out = append(out, byte(x)|0x80)
		x >>= 7
	}
	return append(out, byte(x))
}

func tag(field, wt int) []byte { return encVarint(uint64(field<<3 | wt)) }

func ld(field int, b []byte) []byte {
	out := tag(field, 2)
	out = append(out, encVarint(uint64(len(b)))...)
	return append(out, b...)
}

func vi(field int, v uint64) []byte {
	out := tag(field, 0)
	return append(out, encVarint(v)...)
}

func domainMsg(typ int, val string) []byte {
	var b []byte
	b = append(b, vi(1, uint64(typ))...)
	b = append(b, ld(2, []byte(val))...)
	return b
}

func geoSite(code string, ds ...[]byte) []byte {
	var b []byte
	b = append(b, ld(1, []byte(code))...)
	for _, d := range ds {
		b = append(b, ld(2, d)...)
	}
	return b
}

func list(gs ...[]byte) []byte {
	var b []byte
	for _, g := range gs {
		b = append(b, ld(1, g)...)
	}
	return b
}

func buildDB(t *testing.T) *DB {
	t.Helper()
	data := list(geoSite("test",
		domainMsg(2, "example.com"),    // suffix
		domainMsg(3, "full.test"),      // full
		domainMsg(0, "kwmarker"),       // plain substring
		domainMsg(1, "^re\\d+\\.org$"), // regex
	))
	db, err := Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return db
}

func TestParseAndMatch(t *testing.T) {
	db := buildDB(t)
	cases := []struct {
		host string
		want bool
	}{
		{"example.com", true},
		{"a.example.com", true},
		{"notexample.com", false},
		{"full.test", true},
		{"x.full.test", false},
		{"sub.kwmarker.io", true},
		{"re123.org", true},
		{"other.org", false},
	}
	for _, c := range cases {
		got, known := db.Match("test", c.host)
		if !known {
			t.Fatalf("known=false for %s", c.host)
		}
		if got != c.want {
			t.Errorf("Match(%s)=%v want %v", c.host, got, c.want)
		}
	}
	if _, known := db.Match("nonexist", "example.com"); known {
		t.Error("unknown set should report known=false")
	}
	if !db.Has("test") {
		t.Error("Has(test) false")
	}
}
