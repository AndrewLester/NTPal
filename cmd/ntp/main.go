package main

import (
	"errors"
	"log"
	"net"
	"os"
)

var MTU = 1300

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

	// udp, err := net.ListenPacket("udp", fmt.Sprintf("%s:%s", host, port))
	// if err != nil {
	// 	log.Fatalf("can't listen on %s/udp: %s", port, err)
	// }

	// handleUDP(udp)
}

func handleUDP(c net.PacketConn) {
	packet := make([]byte, MTU)

	for {
		n, addr, err := c.ReadFrom(packet)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}

			log.Printf("error reading on %s/udp: %s", addr.String(), err)
			continue
		}

		c.WriteTo(packet[:n], addr)
	}
}
