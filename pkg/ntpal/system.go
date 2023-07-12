package ntp

import (
	"math"

	"github.com/AndrewLester/ntpal/internal/system/adjtime"
	"github.com/AndrewLester/ntpal/internal/system/settimeofday"
	"golang.org/x/sys/unix"
)

func GetSystemTime() NTPTimestampEncoded {
	var unixTime unix.Timespec
	unix.ClockGettime(unix.CLOCK_REALTIME, &unixTime)
	return (UnixToNTPTimestampEncoded(unixTime))
}

func stepTime(offset float64) {
	systemTime := GetSystemTime()
	ntpTime := DoubleToNTPTimestampEncoded(offset) + systemTime

	Sec := int64(ntpTime >> 32)
	Usec := int32(math.Round(float64(int64(ntpTime)-(Sec<<
		32)) / float64(eraLength) * 1e6))
	Sec -= unixEraOffset

	info("CURRENT:", NTPTimestampToTime(systemTime), "STEPPING TO:", NTPTimestampToTime(ntpTime), "OFFSET WAS:", offset)

	err := settimeofday.Settimeofday(Sec, Usec)
	if err != nil {
		info("SETTIMEOFDAYERR:", err)
	}
}

func adjustTime(offset float64) {
	if offset == 0 {
		return
	}

	sign := math.Copysign(1, offset)
	ntpTime := DoubleToNTPTimestampEncoded(math.Abs(offset))

	Sec := int64(ntpTime>>32) * int64(sign)
	Usec := int32(math.Round(float64(int64(ntpTime)-(Sec<<
		32))/float64(eraLength)*1e6)) * int32(sign)
	debug("Adjtime: ", Sec, Usec)

	for Usec < 0 {
		Sec -= 1
		Usec += 1e6
	}

	err := adjtime.Adjtime(Sec, Usec)
	if err != nil {
		info("ADJTIME ERROR:", err, "offset:", offset)
	}
}
