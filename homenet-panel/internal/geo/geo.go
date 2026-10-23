package geo

import (
	"os"
	"regexp"
	"strings"
)

// DB 是解码后的 geosite 数据（v2ray geosite.dat 格式）。
type DB struct {
	sets map[string][]entry
}

type entry struct {
	typ int // 0=Plain(子串) 1=Regex 2=Domain(后缀) 3=Full(精确)
	val string
}

// readVarint 读取一个 protobuf varint。
func readVarint(b []byte, i int) (uint64, int) {
	var x uint64
	var s uint
	for i < len(b) {
		c := b[i]
		i++
		if c < 0x80 {
			x |= uint64(c) << s
			return x, i
		}
		x |= uint64(c&0x7f) << s
		s += 7
	}
	return x, i
}

// fields 遍历 protobuf 消息，回调 (fieldNum, wireType, value)：
// wireType 2 时 value 为字节切片；wireType 0 时 value 为 nil 但 number 有效。
func fields(b []byte, fn func(num int, wt int, data []byte, numVal uint64)) {
	i := 0
	for i < len(b) {
		key, ni := readVarint(b, i)
		if ni == i {
			return
		}
		i = ni
		num := int(key >> 3)
		wt := int(key & 7)
		switch wt {
		case 0: // varint
			v, ni2 := readVarint(b, i)
			i = ni2
			fn(num, wt, nil, v)
		case 2: // length-delimited
			ln, ni2 := readVarint(b, i)
			i = ni2
			if i+int(ln) > len(b) {
				return
			}
			fn(num, wt, b[i:i+int(ln)], 0)
			i += int(ln)
		case 1: // fixed64
			i += 8
		case 5: // fixed32
			i += 4
		default:
			return
		}
	}
}

// parseGeoSite 解析一条 GeoSite{ code=1, domains=2 }。
func parseGeoSite(b []byte) (string, []entry) {
	code := ""
	var es []entry
	fields(b, func(num int, wt int, data []byte, _ uint64) {
		if num == 1 && wt == 2 {
			code = string(data)
		} else if num == 2 && wt == 2 {
			var typ int
			var val string
			fields(data, func(n2 int, w2 int, d2 []byte, v2 uint64) {
				if n2 == 1 && w2 == 0 {
					typ = int(v2)
				} else if n2 == 2 && w2 == 2 {
					val = strings.ToLower(string(d2))
				}
			})
			if val != "" {
				es = append(es, entry{typ: typ, val: val})
			}
		}
	})
	return strings.ToLower(code), es
}

// Load 读取并解析 geosite.dat。data 为文件原始字节。
func Load(path string) (*DB, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

// Parse 从字节解析 GeoSiteList{ entry=1 }。
func Parse(b []byte) (*DB, error) {
	db := &DB{sets: map[string][]entry{}}
	fields(b, func(num int, wt int, data []byte, _ uint64) {
		if num == 1 && wt == 2 {
			code, es := parseGeoSite(data)
			if code != "" && len(es) > 0 {
				db.sets[code] = append(db.sets[code], es...)
			}
		}
	})
	return db, nil
}

// Has 判断是否包含某 geosite 分类。
func (db *DB) Has(name string) bool {
	if db == nil {
		return false
	}
	_, ok := db.sets[strings.ToLower(name)]
	return ok
}

// Names 返回已知分类列表。
func (db *DB) Names() []string {
	out := make([]string, 0, len(db.sets))
	for k := range db.sets {
		out = append(out, k)
	}
	return out
}

// Match 判断 host 是否属于 geosite:name。返回 (match, known)。
func (db *DB) Match(name, host string) (bool, bool) {
	if db == nil {
		return false, false
	}
	es, ok := db.sets[strings.ToLower(name)]
	if !ok {
		return false, false
	}
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	for _, e := range es {
		switch e.typ {
		case 0: // Plain：子串
			if strings.Contains(h, e.val) {
				return true, true
			}
		case 2: // Domain：后缀（含自身）
			if h == e.val || strings.HasSuffix(h, "."+e.val) {
				return true, true
			}
		case 3: // Full：精确
			if h == e.val {
				return true, true
			}
		case 1: // Regex
			if re, err := regexp.Compile(e.val); err == nil && re.MatchString(h) {
				return true, true
			}
		}
	}
	return false, true
}
