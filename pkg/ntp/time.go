package ntp

import (
	"math"
	"os"
	"time"

	"github.com/AndrewLester/ntp/internal/system/adjtime"
	"github.com/AndrewLester/ntp/internal/system/settimeofday"
	"golang.org/x/sys/unix"
)

func stepTime(offset float64) {
	var unixTime unix.Timeval
	unix.Gettimeofday(&unixTime)
	old := unixTime

	ntpTime := DoubleToNTPTimestampEncoded(offset) + UnixToNTPTimestampEncoded(unixTime)

	Sec := int64(ntpTime >> 32)
	Usec := int32(math.Round(float64(int64(ntpTime)-(Sec<<
		32)) / float64(eraLength) * 1e6))
	Sec -= unixEraOffset

	var now unix.Timeval
	unix.Gettimeofday(&now)
	info("CURRENT:", UnixToTime(old), "STEPPING TO:", NTPTimestampToTime(ntpTime), "OFFSET WAS:", offset)
	if os.Getenv("ENABLED") == "1" {
		err := settimeofday.Settimeofday(Sec, Usec)
		if err != nil {
			info("SETTIMEOFDAYERR:", err)
		}
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
		32)) / float64(eraLength) * 1e6))

	if sign < 0 && Sec == 0 {
		Sec = -1
		Usec += 1e6
	}

	info("Adjust time:", Sec, Usec)

	if os.Getenv("ENABLED") == "1" {
		err := adjtime.Adjtime(Sec, Usec)
		if err != nil {
			info("ADJTIME ERROR:", err, "offset:", offset)
		}
	}
}

func UnixToTime(t unix.Timeval) time.Time {
	return time.Unix(t.Unix())
}

func UnixToNTPTimestampEncoded(time unix.Timeval) NTPTimestampEncoded {
	return uint64((time.Sec+unixEraOffset)<<32) +
		uint64(float64(time.Usec)/1e6*float64(eraLength))
}

func DoubleToNTPTimestampEncoded(offset float64) NTPTimestampEncoded {
	return NTPTimestampEncoded(offset * float64(eraLength))
}

func NTPTimestampEncodedToDouble(ntpTimestamp NTPTimestampEncoded) float64 {
	return float64(ntpTimestamp) / float64(eraLength)
}

func NTPTimestampDifferenceToDouble(difference float64) float64 {
	return difference / float64(eraLength)
}

func Log2ToDouble(a int8) float64 {
	if a < 0 {
		return 1.0 / float64(int32(1)<<-a)
	}
	return float64(int32(1) << a)
}

func NTPTimestampToTime(ntpTimestamp NTPTimestampEncoded) time.Time {
	Sec := int64(ntpTimestamp >> 32)
	Usec := int32(math.Round(float64(int64(ntpTimestamp)-(Sec<<
		32)) / float64(eraLength) * 1e6))
	Sec -= unixEraOffset
	return time.Unix(Sec, int64(Usec)*1e3)
}
