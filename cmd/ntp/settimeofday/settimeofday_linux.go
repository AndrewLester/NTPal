//go:build aix || dragonfly || freebsd || (js && wasm) || linux || nacl || netbsd || openbsd || solaris

package settimeofday

import (
	"golang.org/x/sys/unix"
)

func Settimeofday(Sec int64, Usec int32) error {
	timeVal := unix.Timeval{
		Sec:  Sec,
		Usec: int64(Usec),
	}
	return unix.Settimeofday(&timeVal)
}
