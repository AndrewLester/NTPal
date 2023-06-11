package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
)

var MTU = 1300

var PRECISION int8 = -18 /* precision (log2 s)  */

const DEFAULT_CONFIG_PATH = "/etc/ntp.conf"

func main() {
	var config string
	flag.StringVar(&config, "config", DEFAULT_CONFIG_PATH, "Path to the NTP config file.")

	flag.Parse()

	associationConfigs := ParseConfig(config)

	fmt.Println(associationConfigs)

	return

	portStr := os.Getenv("NTP_PORT")
	port, err := strconv.Atoi(portStr)
	if portStr == "" || err != nil {
		port = 1230
	}

	host := os.Getenv("NTP_HOST")

	system := &NTPSystem{
		leap:      NOSYNC,
		stratum:   MAXSTRAT,
		poll:      MINPOLL,
		precision: PRECISION,
	}

	var wg sync.WaitGroup

	addr := net.UDPAddr{
		Port: port,
		IP:   net.ParseIP(host),
	}
	udp, err := net.ListenUDP("udp", &addr)
	if err != nil {
		log.Fatalf("can't listen on %s/udp: %s", port, err)
	}

	associations := []*Association{&Association{}}

	system.SetupAsssociations(associations, &wg)

	wg.Add(1)
	go handleUDP(udp, system, &wg)

	wg.Wait()
}

func handleUDP(c *net.UDPConn, system *NTPSystem, wg *sync.WaitGroup) {
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
