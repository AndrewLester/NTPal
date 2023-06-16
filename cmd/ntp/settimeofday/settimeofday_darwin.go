package settimeofday

import (
	"golang.org/x/sys/unix"
)

func Settimeofday(Sec int64, Usec int32) error {
	timeVal := unix.Timeval{
		Sec:  Sec,
		Usec: Usec,
	}
	return unix.Settimeofday(&timeVal)
}
