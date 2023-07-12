package ntp

type NTPSystem interface {
	GetAssociations() []*Association
}

type Association struct {
	Offset  float64
	Reach byte
	ReceivePacket
}
