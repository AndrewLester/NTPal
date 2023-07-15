package main

import (
	"errors"
	"fmt"
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

var ErrNotRunning = errors.New("daemon ntpald is not running")

var daemonCtx = &daemon.Context{
	PidFileName: PidPath,
	PidFilePerm: 0644,
	LogFileName: LogPath,
	LogFilePerm: 0640,
	WorkDir:     "./",
	Umask:       027,
	Args:        append([]string{daemonName}, os.Args[1:]...),
}

func killDaemon() error {
	daemon, err := daemonCtx.Search()
	if daemon == nil {
		if err == nil {
			err = ErrNotRunning
		}
		return err
	}

	err = syscall.Kill(daemon.Pid, syscall.SIGTERM)
	if err != nil {
		return err
	}

	return nil
}
