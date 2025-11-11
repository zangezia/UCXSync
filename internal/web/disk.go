//go:build linux

package web

import "syscall"

// getDiskSpace returns free and total disk space in bytes for a given path
func getDiskSpace(path string) (freeGB, totalGB float64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}

	totalGB = float64(stat.Blocks*uint64(stat.Bsize)) / 1024 / 1024 / 1024
	freeGB = float64(stat.Bavail*uint64(stat.Bsize)) / 1024 / 1024 / 1024

	return freeGB, totalGB, nil
}
