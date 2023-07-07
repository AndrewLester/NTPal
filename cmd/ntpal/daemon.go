package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"syscall"

	"github.com/sevlyar/go-daemon"
)

const daemonName = "ntpald"

var daemonCtx = &daemon.Context{
	PidFileName: fmt.Sprintf("/var/run/%s.pid", daemonName),
	PidFilePerm: 0644,
	LogFileName: fmt.Sprintf("/var/log/%s.log", daemonName),
	LogFilePerm: 0640,
	WorkDir:     "./",
	Umask:       027,
	Args:        append([]string{daemonName}, os.Args[1:]...),
}

func killDaemon() {
	data, err := os.ReadFile(daemonCtx.PidFileName)
	if err != nil {
		log.Fatal("Error reading PID file.")
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		log.Fatal("Error reading PID file.")
	}

	err = syscall.Kill(pid, syscall.SIGTERM)
	if err != nil {
		log.Fatal("Couldn't stop ntpal daemon.")
	}
}
