SHELL := /bin/bash -f

.PHONY: all

all:
	go build -o bin/ntp github.com/AndrewLester/ntpal/cmd/ntp && go build -o bin/ntp-report github.com/AndrewLester/ntpal/cmd/ntp-report

clean:
	rm bin/ntp; rm bin/ntp-report
