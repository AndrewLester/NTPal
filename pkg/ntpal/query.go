package ntpal

import (
	"errors"
	"net"
	"time"

	"github.com/AndrewLester/ntpal/internal/ntp"
)

type QueryResult struct {
	Offset float64
	Err    float64
}

var (
	ErrNoSync     = errors.New("server is not synchronized")
	ErrNoResponse = errors.New("server did not respond")
)

func (system *NTPalSystem) Query(address string, messages int) (*QueryResult, error) {
	system.query = true

	hostAddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(system.host, system.port))
	if err != nil {
		return nil, err
	}
	system.address = hostAddr

	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(address, ntp.Port))
	if err != nil {
		return nil, err
	}
	association := &Association{
		hmode: ntp.CLIENT,
		Association: ntp.Association{
			Hpoll: 0,
			ReceivePacket: ntp.ReceivePacket{
				Srcaddr: addr,
				Dstaddr: system.address,
				Version: VERSION,
				Keyid:   0,
			},
		},
	}
	system.clear(association, INIT)
	system.associations = append(system.associations, association)

	system.listen()
	go system.setupServer()

	responded := false
	for i := 0; i < messages; i++ {
		system.pollPeer(association)
		select {
		case <-system.filtered:
			// Exit early if the association's state is not synced
			if association.Leap == NOSYNC {
				return nil, ErrNoSync
			}

			responded = true
			system.FilteredProgress <- 0
		case <-time.After(time.Duration(1) * time.Second):
			system.FilteredProgress <- 0
		}
	}

	if !responded {
		return nil, ErrNoResponse
	}

	minDelayStage := association.f[0]
	for _, stage := range association.f {
		if stage.delay < minDelayStage.delay {
			minDelayStage = stage
		}
	}

	// lambda is error in a given sample's offset
	lambda := association.Rootdelay/2 + association.Rootdisp + minDelayStage.delay

	return &QueryResult{Offset: minDelayStage.offset, Err: lambda}, nil
}
