package versionup

import "testing"

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.2.0", "v1.2.0", 0},
		{"1.2.0", "v1.2.0", 0}, // 前缀 v 不影响
		{"v1.2.0", "v1.3.0", -1},
		{"v1.10.0", "v1.9.0", 1}, // 按数值而非字典序
		{"v1.3.0", "v1.3.0-rc1", 1},
		{"v1.3.0-rc1", "v1.3.0", -1},
		{"v1.3.0-rc1", "v1.3.0-rc2", -1},
		{"v1.3.0-rc.1", "v1.3.0-rc.10", -1}, // 数字标识符按数值
		{"v1.2.0", "v1.2.0+build.1", 0},     // 构建元数据忽略
	}
	for _, c := range cases {
		got, err := Compare(c.a, c.b)
		if err != nil {
			t.Fatalf("Compare(%q,%q) error: %v", c.a, c.b, err)
		}
		if got != c.want {
			t.Errorf("Compare(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestCompare_Invalid(t *testing.T) {
	for _, s := range []string{"", "v", "1.2.3.4", "abc", "v1.x.0"} {
		if _, err := Compare(s, "1.0.0"); err == nil {
			t.Errorf("Compare(%q,...) expected error, got nil", s)
		}
	}
}

func TestNeedsUpgrade(t *testing.T) {
	cases := []struct {
		current, latest string
		stable          bool
		want            bool
	}{
		{"v1.2.0", "v1.3.0", true, true},
		{"v1.2.0", "v1.3.0", false, false}, // 非稳定不升级
		{"v1.3.0", "v1.3.0-rc1", true, false},
		{"v1.3.0", "v1.3.0", true, false},  // 相同不升级
		{"v1.4.0", "v1.3.0", true, false},  // 降级不升级
		{"v1.2.0", "1.2.1", true, true},
	}
	for _, c := range cases {
		if got := NeedsUpgrade(c.current, c.latest, c.stable); got != c.want {
			t.Errorf("NeedsUpgrade(%q,%q,%v) = %v, want %v", c.current, c.latest, c.stable, got, c.want)
		}
	}
}
