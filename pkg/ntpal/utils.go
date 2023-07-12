package ntp

import (
	"log"
	"os"
)

func info(args ...any) {
	if isInfo() {
		log.Println(args...)
	}
}

func debug(args ...any) {
	if isDebug() {
		log.Println(args...)
	}
}

func isInfo() bool {
	return os.Getenv("INFO") == "1"
}

func isDebug() bool {
	return os.Getenv("DEBUG") == "1"
}

func isSymmetricEnabled() bool {
	return os.Getenv("SYMMETRIC") == "1"
}
