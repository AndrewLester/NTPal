SHELL := /bin/bash -f

.PHONY: all

all:
	go build -o bin/ntp github.com/AndrewLester/ntp/cmd/ntp && go build -o bin/ntp-report github.com/AndrewLester/ntp/cmd/ntp-report

clean:
	rm bin/ntp; rm bin/ntp-report
