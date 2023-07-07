package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/AndrewLester/ntpal/pkg/ntp"
	"github.com/sevlyar/go-daemon"
)

const defaultConfigPath = "/etc/ntp.conf"
const defaultDriftPath = "/etc/ntp.drift"

func main() {
	var config string
	var drift string
	var query string
	var noDaemon bool
	flag.StringVar(&config, "config", defaultConfigPath, "Path to the NTP config file.")
	flag.StringVar(&drift, "drift", defaultDriftPath, "Path to the NTP drift file.")
	flag.StringVar(&query, "query", "", "Address to query.")
	flag.StringVar(&query, "q", query, "Address to query.")
	flag.BoolVar(&noDaemon, "no-daemon", false, "Don't run ntpal as a daemon.")
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
		if !noDaemon {
			d, err := daemonCtx.Reborn()
			if err != nil {
				if errors.Is(err, daemon.ErrWouldBlock) {
					killDaemon()
					fmt.Println("Successfully stopped ntpal daemon.")
					return
				}
				log.Fatal("Unable to run: ", err)
			}
			if d != nil {
				fmt.Printf("Daemon process (ntpald, %d) started successfully.\n", d.Pid)
				return
			}
			defer daemonCtx.Release()

			log.Print("- - - - - - - - - - - - - - -")
			log.Print("daemon started", os.Args)
		}

		system.Start()
	}
}
