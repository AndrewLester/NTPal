package ntp

import (
	"errors"
	"log"
	"net"
	"net/rpc"
	"os"

	"github.com/AndrewLester/ntpal/internal/ntp"
)

type NTPalRPCServer struct {
	Socket string
	System *NTPSystem
}

func (s *NTPalRPCServer) Listen() {
	defer s.System.wg.Done()
	
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
	*reply = s.System.GetAssociations()
	return nil
}
