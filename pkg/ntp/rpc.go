package ntp

import (
	"errors"
	"log"
	"net"
	"net/rpc"
	"os"
)

type RPCServer struct {
	Socket string
	System *NTPSystem
}

type RPCAssociation struct {
	SrcAddr *net.UDPAddr
	Offset  float64
}

func (s *RPCServer) Listen() {
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

	log.Println("RPC listening")

	for {
		rpc.Accept(l)
	}
}

func (s *RPCServer) FetchInfo(args int, reply *[]*RPCAssociation) error {
	rpcAssociations := []*RPCAssociation{}
	for _, association := range s.System.associations {
		rpcAssociation := &RPCAssociation{
			SrcAddr: association.srcaddr,
			Offset:  association.offset,
		}
		rpcAssociations = append(rpcAssociations, rpcAssociation)
	}
	info("Fetched, associations:", rpcAssociations)
	*reply = rpcAssociations
	return nil
}
