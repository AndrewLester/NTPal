package main

import (
	"flag"
	"fmt"
	"net"
	"os"

	"github.com/AndrewLester/ntp/pkg/ntp"
)

const DEFAULT_CONFIG_PATH = "/etc/ntp.conf"
const DEFAULT_DRIFT_PATH = "/etc/ntp.drift"

func main() {
	var config string
	var drift string
	var query string
	flag.StringVar(&config, "config", DEFAULT_CONFIG_PATH, "Path to the NTP config file.")
	flag.StringVar(&drift, "drift", DEFAULT_DRIFT_PATH, "Path to the NTP drift file.")
	flag.StringVar(&query, "query", "", "Address to query.")
	flag.StringVar(&query, "q", query, "Address to query.")
	flag.Parse()

	port := os.Getenv("NTP_PORT")
	if port == "" {
		if query != "" {
			port = "0" // System will choose random port
		} else {
			port = "123"
		}
	}
	host := os.Getenv("NTP_HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	system := ntp.NewNTPSystem(host, port, config, drift)

	if query != "" {
		offset, delay := system.Query(query)
		addr, _ := net.ResolveIPAddr("ip", query)
		fmt.Println(offset, "+/-", delay, query, addr.String())
	} else {
		system.Start()
	}
}
