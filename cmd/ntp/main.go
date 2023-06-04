package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
)

var MTU = 1300

var PRECISION int8 = -18 /* precision (log2 s)  */

func main() {
	// in other programming languages, this might look like:
	//    s = socket(AF_INET, SOCK_DGRAM, IPPROTO_UDP)
	//    s.bind("fly-global-services", port)

	port := os.Getenv("NTP_PORT")
	if port == "" {
		port = "1230"
	}

	host := os.Getenv("NTP_HOST")
	if host == "" {
		host = ""
	}

	system := &NTPSystem{
		leap:      NOSYNC,
		stratum:   MAXSTRAT,
		poll:      MINPOLL,
		precision: PRECISION,
	}

	var wg sync.WaitGroup

	udp, err := net.ListenPacket("udp", fmt.Sprintf("%s:%s", host, port))
	if err != nil {
		log.Fatalf("can't listen on %s/udp: %s", port, err)
	}

	servers := []string{"time.apple.com", "time.cloudflare.com"}

	for _, server := range servers {
		SetupAsssociation(server, &wg)
	}

	wg.Add(1)
	go handleUDP(udp, system, &wg)

	wg.Wait()
}

func handleUDP(c net.PacketConn, system *NTPSystem, wg *sync.WaitGroup) {
	defer wg.Done()

	packet := make([]byte, MTU)

	for {
		_, addr, err := c.ReadFrom(packet)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}

			log.Printf("error reading on %s/udp: %s", addr.String(), err)
			continue
		}

		recvPacket, err := DecodeRecvPacket(packet, addr, c)
		if err != nil {
			log.Printf("Error reading packet: %v", err)
		}
		recvPacket.dst = GetSystemTime()
		reply := system.Receive(*recvPacket)
		if reply == nil {
			log.Printf("Dropping packet:", reply.org)
			return
		}
		encoded := EncodeTransmitPacket(*reply)
		c.WriteTo(encoded, addr)
	}
}
