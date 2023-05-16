.PHONY: all

all:
	go build -o bin/ntp cmd/ntp/*.go && go build -o bin/ntp-report cmd/ntp-report/*.go
