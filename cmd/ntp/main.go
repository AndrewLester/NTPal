package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"sync"
	"time"
)

var MTU = 1300

var PRECISION int8 = -18 /* precision (log2 s)  */

const DEFAULT_CONFIG_PATH = "/etc/ntp.conf"

func main() {
	var config string
	flag.StringVar(&config, "config", DEFAULT_CONFIG_PATH, "Path to the NTP config file.")
	flag.Parse()

	rand.Seed(time.Now().UnixNano())

	associationConfigs := ParseConfig(config)

	fmt.Println("Association configs:", associationConfigs)

	port := os.Getenv("NTP_PORT")
	if port == "" {
		port = "1230"
	}

	host := os.Getenv("NTP_HOST")
	if host == "" {
		host = "127.0.0.1"
	}

	address, err := net.ResolveUDPAddr("udp", host+":"+port)
	if err != nil {
		log.Fatal("Could not resolve NTP_HOST + NTP_PORT")
	}

	system := &NTPSystem{
		address:   address,
		leap:      NOSYNC,
		stratum:   MAXSTRAT,
		poll:      MINPOLL,
		precision: PRECISION,
	}
	associations := system.CreateAssociations(associationConfigs)

	var wg sync.WaitGroup

	udp, err := net.ListenUDP("udp", address)
	if err != nil {
		log.Fatalf("can't listen on %d/udp: %s", port, err)
	}

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
		if reply == nil && system.hmode != CLIENT {
			log.Printf("Dropping packet: %d", reply.org)
			return
		}
		encoded := EncodeTransmitPacket(*reply)
		c.WriteTo(encoded, addr)
	}
}