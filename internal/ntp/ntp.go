package ntp

type System struct {
	clock Clock
}

type Clock struct {
	T      NTPTimestampEncoded
	State  int
	Offset float64
	Last   float64
	Count  int32
	Freq   float64
	Jitter float64
	Wander float64
}

type Association struct {
	Offset  float64
	Jitter float64
	Reach byte
	Update float64  // clock.t of the last collected sample
	ReceivePacket
}

const (
	Port = 123  // NTP port number
)