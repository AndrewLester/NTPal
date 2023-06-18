package main

import (
	"flag"
	"math/rand"
	"os"
	"time"

	"github.com/AndrewLester/ntp/pkg/ntp"
)

const DEFAULT_CONFIG_PATH = "/etc/ntp.conf"
const DEFAULT_DRIFT_PATH = "/etc/ntp.drift"

func main() {
	var config string
	var drift string
	flag.StringVar(&config, "config", DEFAULT_CONFIG_PATH, "Path to the NTP config file.")
	flag.StringVar(&drift, "drift", DEFAULT_DRIFT_PATH, "Path to the NTP drift file.")
	flag.Parse()

	rand.Seed(time.Now().UnixNano())

	port := os.Getenv("NTP_PORT")
	if port == "" {
		port = "123"
	}
	host := os.Getenv("NTP_HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	system := ntp.NewNTPSystem(host, port, config, drift)

	system.Start()
}
