package ntp

type System struct {
	clock Clock
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