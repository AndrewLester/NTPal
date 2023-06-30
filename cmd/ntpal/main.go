package main

import (
	"flag"
	"os"

	"github.com/AndrewLester/ntpal/pkg/ntp"
)

const defaultConfigPath = "/etc/ntp.conf"
const defaultDriftPath = "/etc/ntp.drift"

func main() {
	var config string
	var drift string
	var query string
	flag.StringVar(&config, "config", defaultConfigPath, "Path to the NTP config file.")
	flag.StringVar(&drift, "drift", defaultDriftPath, "Path to the NTP drift file.")
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
		handleQueryCommand(system, query)
	} else {
		system.Start()
	}
}
