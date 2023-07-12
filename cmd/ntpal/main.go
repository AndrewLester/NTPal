package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/AndrewLester/ntpal/pkg/ntpal"
)

const defaultConfigPath = "/etc/ntp.conf"
const defaultDriftPath = "/etc/ntp.drift"

var socketPath = fmt.Sprintf("/var/%s.sock", daemonName)

const queryMessages = 5

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

	system := ntpal.NewSystem(host, port, config, drift, socketPath)

	if query != "" {
		handleQueryCommand(system, query, queryMessages)
	} else {
		process, _ := daemonCtx.Search()

		if !noDaemon {
			if process != nil {
				handleNTPalUI(SocketPath)
				return
			}

			process, err := daemonCtx.Reborn()
			if err != nil {
				if errors.Is(err, os.ErrPermission) {
					fmt.Printf("You do not have permission to start the daemon: %v\n", err)
					os.Exit(1)
				}
				fmt.Printf("Error: unable to run: %v\n", err)
				os.Exit(1)
			}

			// Parent process
			if process != nil {
				fmt.Printf("Daemon process (ntpald) started successfully with PID: %d\n", process.Pid)
				return
			}

			// Child process
			defer daemonCtx.Release()

			log.Print(strings.Repeat("*", 20))
			log.Print("ntpald started", os.Args)
		} else if process != nil {
			fmt.Println("Stop the ntpal daemon via `ntpal -s` before running in no-daemon mode.")
			os.Exit(1)
		}

		system.Start()
	}
}
