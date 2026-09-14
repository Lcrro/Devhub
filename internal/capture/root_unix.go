//go:build !windows

package capture

import "os"

func isElevatedRoot() bool { return os.Geteuid() == 0 }
