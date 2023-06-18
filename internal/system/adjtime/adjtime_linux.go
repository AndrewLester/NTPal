//go:build aix || dragonfly || freebsd || (js && wasm) || linux || nacl || netbsd || openbsd || solaris

package adjtime

import (
	"golang.org/x/sys/unix"
)

func Adjtime(Sec int64, Usec int32) error {
	timeVal := unix.Timeval{
		Sec:  Sec,
		Usec: int64(Usec),
	}
	buf := &unix.Timex{
		Time:  timeVal,
		Modes: unix.ADJ_SETOFFSET,
	}
	_, err := unix.Adjtimex(buf)
	return err
}
