package versionup

import (
	"fmt"
	"strconv"
	"strings"
)

// Compare 比较两个语义化版本号 a 与 b（允许前缀 "v"/"V"，忽略构建元数据 "+xxx"）。
// 返回 -1（a<b）、0（相等）、1（a>b）。
// 预发布版本（如 1.5.0-rc1）按 SemVer 规则低于同核心版本的正式版。
func Compare(a, b string) (int, error) {
	va, err := parseVersion(a)
	if err != nil {
		return 0, fmt.Errorf("parse %q: %w", a, err)
	}
	vb, err := parseVersion(b)
	if err != nil {
		return 0, fmt.Errorf("parse %q: %w", b, err)
	}
	return va.compare(vb), nil
}

// NeedsUpgrade 判断从 current 升级到 latest 是否必要。
// 仅当 latest > current 且 stable 为真时返回 true。
func NeedsUpgrade(current, latest string, stable bool) bool {
	if !stable {
		return false
	}
	c, err := Compare(current, latest)
	if err != nil {
		return false
	}
	return c < 0
}

// version 表示一个解析后的语义化版本。
type version struct {
	major, minor, patch uint64
	pre                 []preID
}

// preID 是预发布标识符：数字标识符按数值比较，非数字按字典序。
type preID struct {
	str   string
	num   uint64
	isNum bool
}

// parseVersion 解析 "v1.2.3-rc1+build" 形式，返回核心版本与预发布标识符。
func parseVersion(s string) (version, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")

	// 忽略构建元数据（不影响优先级）。
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}

	var core, pre string
	if i := strings.IndexByte(s, '-'); i >= 0 {
		core = s[:i]
		pre = s[i+1:]
	} else {
		core = s
	}

	parts := strings.Split(core, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return version{}, fmt.Errorf("invalid version core %q", core)
	}
	nums := [3]uint64{}
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return version{}, fmt.Errorf("invalid number %q", p)
		}
		nums[i] = n
	}

	v := version{major: nums[0], minor: nums[1], patch: nums[2]}

	if pre != "" {
		for _, id := range strings.Split(pre, ".") {
			if id == "" {
				return version{}, fmt.Errorf("empty prerelease identifier")
			}
			if n, err := strconv.ParseUint(id, 10, 64); err == nil {
				v.pre = append(v.pre, preID{num: n, isNum: true, str: id})
			} else {
				v.pre = append(v.pre, preID{str: id, isNum: false})
			}
		}
	}
	return v, nil
}

// compare 按 SemVer 优先级比较两个已解析版本。
func (v version) compare(o version) int {
	if c := cmpUint(v.major, o.major); c != 0 {
		return c
	}
	if c := cmpUint(v.minor, o.minor); c != 0 {
		return c
	}
	if c := cmpUint(v.patch, o.patch); c != 0 {
		return c
	}
	// 核心版本相等：无预发布 > 有预发布。
	switch {
	case len(v.pre) == 0 && len(o.pre) == 0:
		return 0
	case len(v.pre) == 0:
		return 1
	case len(o.pre) == 0:
		return -1
	}
	// 都有预发布：逐标识符比较。
	n := len(v.pre)
	if len(o.pre) < n {
		n = len(o.pre)
	}
	for i := 0; i < n; i++ {
		a, b := v.pre[i], o.pre[i]
		switch {
		case a.isNum && b.isNum:
			if c := cmpUint(a.num, b.num); c != 0 {
				return c
			}
		case a.isNum && !b.isNum:
			return -1 // 数字标识符优先级低于非数字
		case !a.isNum && b.isNum:
			return 1
		default:
			if c := strings.Compare(a.str, b.str); c != 0 {
				return c
			}
		}
	}
	return cmpInt(len(v.pre), len(o.pre))
}

func cmpUint(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
