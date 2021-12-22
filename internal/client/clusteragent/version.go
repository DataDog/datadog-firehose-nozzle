package clusteragent

import "fmt"

type Version struct {
	Major  int64
	Minor  int64
	Patch  int64
	Pre    string
	Meta   string
	Commit string
}

// GetNumber returns a string containing version numbers only, e.g. `0.0.0`
func (v *Version) GetNumber() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func (v *Version) String() string {
	ver := v.GetNumber()
	if v.Pre != "" {
		ver = fmt.Sprintf("%s-%s", ver, v.Pre)
	}
	if v.Meta != "" {
		ver = fmt.Sprintf("%s+%s", ver, v.Meta)
	}
	if v.Commit != "" {
		if v.Meta != "" {
			ver = fmt.Sprintf("%s.commit.%s", ver, v.Commit)
		} else {
			ver = fmt.Sprintf("%s+commit.%s", ver, v.Commit)
		}
	}

	return ver
}
