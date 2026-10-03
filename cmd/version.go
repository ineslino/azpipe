package cmd

import (
	"fmt"
	"runtime/debug"
)

var buildVersion = "dev"
var buildCommit string

func versionString() string {
	commit, modified := buildCommit, "unknown"
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				if commit == "" {
					commit = setting.Value
				}
			case "vcs.modified":
				modified = setting.Value
			}
		}
	}
	if commit == "" {
		commit = "unknown"
	}
	return fmt.Sprintf("%s (commit %s; modified=%s)", buildVersion, commit, modified)
}
