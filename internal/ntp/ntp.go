package ntp

type System struct {
	Clock *Clock
}

type Clock struct {
	T      TimestampEncoded
	State  int
	Offset float64
	Last   float64
	Count  int32
	Freq   float64
	Jitter float64
	Wander float64
}

type Association struct {
	Offset        float64
	Jitter        float64
	Reach         byte
	Hostname      string
	IburstEnabled bool
	BurstEnabled  bool
	Rootdelay     float64
	Rootdisp      float64
	Delay         float64
	Disp          float64

	Update float64 // clock.t of the last collected sample
	Hpoll  int8

	ReceivePacket
}

type TimestampEncoded = uint64

type ShortEncoded = uint32

type Digest = uint32

type Mode byte

const (
	RESERVED Mode = iota
	SYMMETRIC_ACTIVE
	SYMMETRIC_PASSIVE
	CLIENT
	SERVER
	BROADCAST_SERVER
	BROADCAST_CLIENT // Also NTP_CONTROL_MESSAGE?
	RESERVED_PRIVATE_USE
)

const (
	Port = "123" // NTP port number
)
