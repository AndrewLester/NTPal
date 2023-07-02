SHELL := /bin/bash -f

.PHONY: all

all:
	go build -o bin/ntpal github.com/AndrewLester/ntpal/cmd/ntpal && go build -o bin/ntpal-report github.com/AndrewLester/ntpal/cmd/ntpal-report

clean:
	rm bin/ntpal; rm bin/ntpal-report
