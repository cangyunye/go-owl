//go:build !darwin

package handler

import (
	"os"
	"time"
)

func getBirthTime(fi os.FileInfo) time.Time {
	return fi.ModTime()
}
