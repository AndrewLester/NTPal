package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/AndrewLester/ntpal/pkg/ntp"
	"github.com/sevlyar/go-daemon"
)

const defaultConfigPath = "/etc/ntp.conf"
const defaultDriftPath = "/etc/ntp.drift"

var socketPath = fmt.Sprintf("/var/%s.sock", daemonName)

func main() {
	var config string
	var drift string
	var query string
	var noDaemon bool
	var stop bool
	flag.StringVar(&config, "config", defaultConfigPath, "Path to the NTP config file.")
	flag.StringVar(&drift, "drift", defaultDriftPath, "Path to the NTP drift file.")
	flag.StringVar(&query, "query", "", "Address to query.")
	flag.StringVar(&query, "q", query, "Address to query.")
	flag.BoolVar(&noDaemon, "no-daemon", false, "Don't run ntpal as a daemon.")
	flag.BoolVar(&stop, "stop", false, "Stop the ntpal daemon.")
	flag.BoolVar(&stop, "s", stop, "Stop the ntpal daemon.")
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

	system := ntp.NewNTPSystem(host, port, config, drift, socketPath)

	if query != "" {
		handleQueryCommand(system, query)
	} else {
		if !noDaemon {
			process, err := daemonCtx.Reborn()
			if err != nil {
				if errors.Is(err, daemon.ErrWouldBlock) {
					handleNTPalUI(socketPath)
					return
				}
				if errors.Is(err, os.ErrPermission) {
					log.Fatal("You do not have permission to start the daemon. Try re-running with sudo.")
					return
				}
				log.Fatal("Unable to run: ", err)
			}
			if process != nil {
				fmt.Printf("Daemon process (ntpald) started successfully with PID: %d\n", process.Pid)
				return
			}
			defer daemonCtx.Release()

			log.Print(strings.Repeat("*", 20))
			log.Print("ntpald started", os.Args)
		} else {
			process, _ := daemonCtx.Search()
			if process != nil {
				log.Fatal("Stop the ntpal daemon via `ntpal -s` before running in no-daemon mode.")
			}
		}

		system.Start()
	}
}
