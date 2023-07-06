package ntp

import (
	"fmt"
	"os"
)

func info(args ...any) {
	if isInfo() {
		fmt.Println(args...)
	}
}

func debug(args ...any) {
	if isDebug() {
		fmt.Println(args...)
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
