package handler

import (
	"os"
	"syscall"
	"time"
)

func getBirthTime(fi os.FileInfo) time.Time {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fi.ModTime()
	}
	return time.Unix(st.Birthtimespec.Sec, st.Birthtimespec.Nsec)
}
