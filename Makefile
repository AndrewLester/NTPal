SHELL := /bin/bash -f

.PHONY: all

all:
	go build -o bin/ntpal github.com/AndrewLester/ntpal/cmd/ntpal && go build -o bin/site github.com/AndrewLester/ntpal/cmd/site

clean:
	rm -rf bin
