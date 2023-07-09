package main

import (
	"fmt"
	"log"
	"os"
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
	daemon, err := daemonCtx.Search()
	if err != nil {
		log.Fatalf("Error finding daemon: %v", err)
	}

	err = syscall.Kill(daemon.Pid, syscall.SIGTERM)
	if err != nil {
		log.Fatal("Couldn't stop ntpal daemon.")
	}
}
