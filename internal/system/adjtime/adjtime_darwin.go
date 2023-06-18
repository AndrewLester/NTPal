package adjtime

import (
	"golang.org/x/sys/unix"
)

func Adjtime(Sec int64, Usec int32) error {
	timeVal := unix.Timeval{
		Sec:  Sec,
		Usec: Usec,
	}
	return unix.Adjtime(&timeVal, nil)
}
