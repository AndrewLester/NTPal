package ntp

import (
	"errors"
	"math/rand"
	"net"
	"time"
)

type QueryResult struct {
	Offset float64
	Err    float64
}

var (
	ErrNoSync     = errors.New("server is not synchronized")
	ErrNoResponse = errors.New("server did not respond")
)

func (system *NTPSystem) Query(address string, messages int) (*QueryResult, error) {
	system.query = true

	rand.Seed(time.Now().UnixNano())

	hostAddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(system.host, system.port))
	if err != nil {
		return nil, err
	}
	system.address = hostAddr

	addr, err := net.ResolveUDPAddr("udp", address+":123")
	if err != nil {
		return nil, err
	}
	association := &Association{
		hmode: CLIENT,
		hpoll: 0,
		ReceivePacket: ReceivePacket{
			srcaddr: addr,
			dstaddr: system.address,
			version: VERSION,
			keyid:   0,
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
			if association.leap == NOSYNC {
				return nil, ErrNoSync
			}

			responded = true
			system.ProgressFiltered <- 0
		case <-time.After(time.Duration(1) * time.Second):
			system.ProgressFiltered <- 0
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
