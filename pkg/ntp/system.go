package ntp

import "golang.org/x/sys/unix"

func GetSystemTime() NTPTimestampEncoded {
	var unixTime unix.Timespec
	unix.ClockGettime(unix.CLOCK_REALTIME, &unixTime)
	return (UnixToNTPTimestampEncoded(unixTime))
}
