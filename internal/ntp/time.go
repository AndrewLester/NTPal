package ntp

import (
	"math"
	"time"

	"golang.org/x/sys/unix"
)

const (
	EraLength     int64   = 4_294_967_296 // 2^32
	UnixEraOffset int64   = 2_208_988_800 // 1970 - 1900 in seconds
	ShortLength   float64 = 65536         // 2^16
)

func UnixToTime(t unix.Timeval) time.Time {
	return time.Unix(t.Unix())
}

func UnixToNTPTimestampEncoded(time unix.Timespec) TimestampEncoded {
	return TimestampEncoded((time.Sec+UnixEraOffset)<<32) +
		TimestampEncoded(float64(time.Nsec)/1e9*float64(EraLength))
}

func DoubleToNTPTimestampEncoded(offset float64) TimestampEncoded {
	return TimestampEncoded(offset * float64(EraLength))
}

func NTPTimestampEncodedToDouble(ntpTimestamp TimestampEncoded) float64 {
	return float64(ntpTimestamp) / float64(EraLength)
}

func NTPTimestampDifferenceToDouble(difference int64) float64 {
	return float64(difference) / float64(EraLength)
}

func Log2ToDouble(a int8) float64 {
	if a < 0 {
		return 1.0 / float64(int64(1)<<-a)
	}
	return float64(int64(1) << a)
}

func NTPTimestampToTime(ntpTimestamp TimestampEncoded) time.Time {
	Sec := int64(ntpTimestamp >> 32)
	Usec := int32(math.Round(float64(int64(ntpTimestamp)-(Sec<<
		32)) / float64(EraLength) * 1e6))
	Sec -= UnixEraOffset
	return time.Unix(Sec, int64(Usec)*1e3)
}
