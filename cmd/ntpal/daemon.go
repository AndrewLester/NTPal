package main

import (
	"fmt"
	"log"
	"os"
	"syscall"

	"github.com/sevlyar/go-daemon"
)

const daemonName = "ntpald"

var (
	SocketPath = fmt.Sprintf("/var/%s.sock", daemonName)
	PidPath    = fmt.Sprintf("/var/run/%s.pid", daemonName)
	LogPath    = fmt.Sprintf("/var/log/%s.log", daemonName)
)

var daemonCtx = &daemon.Context{
	PidFileName: PidPath,
	PidFilePerm: 0644,
	LogFileName: LogPath,
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
