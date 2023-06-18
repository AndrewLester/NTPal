package ntp

import "golang.org/x/sys/unix"

func GetSystemTime() NTPTimestampEncoded {
	var unixTime unix.Timeval
	unix.Gettimeofday(&unixTime)
	return (UnixToNTPTimestampEncoded(unixTime))
}
