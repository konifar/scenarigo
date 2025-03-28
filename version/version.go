package version

import (
	"fmt"
	"runtime/debug"
)

var (
	version  string = "0.8.1"
	revision string = "dev"
	info, ok        = debug.ReadBuildInfo()
)

// String returns scenarigo version as string.
func String() string {
	if ok {
		if info.Main.Sum != "" {
			return info.Main.Version
		}
	}
	if revision == "" {
		return fmt.Sprintf("v%s", version)
	}
	return fmt.Sprintf("v%s-%s", version, revision)
}
