//go:build !linux

package web

import "fmt"

// getDiskSpace is a stub for non-Linux platforms (development only)
func getDiskSpace(path string) (freeGB, totalGB float64, err error) {
	return 0, 0, fmt.Errorf("disk space checking only supported on Linux")
}
