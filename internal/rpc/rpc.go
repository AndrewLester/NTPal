package rpc

import (
	"errors"
	"log"
	"net"
	"net/rpc"
	"os"
	"sync"

	"github.com/AndrewLester/ntpal/internal/ntp"
)

type NTPalRPCServer struct {
	Socket          string
	System          *ntp.System
	GetAssociations func() []*ntp.Association
}

func (s *NTPalRPCServer) Listen(wg *sync.WaitGroup) {
	defer wg.Done()

	rpc.Register(s)

	err := os.Remove(s.Socket)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatal("bind error:", err)
	}

	l, e := net.Listen("unix", s.Socket)
	if e != nil {
		log.Fatal("listen error:", e)
	}

	for {
		rpc.Accept(l)
	}
}

func (s *NTPalRPCServer) FetchAssociations(args int, reply *[]*ntp.Association) error {
	*reply = s.GetAssociations()
	return nil
}

func (s *NTPalRPCServer) FetchSystem(args int, reply **ntp.System) error {
	*reply = s.System
	return nil
}
