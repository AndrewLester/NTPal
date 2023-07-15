SHELL := /bin/bash -f

.PHONY: all

all: ntpal site

ntpal:
	go build -o bin/ntpal -ldflags "-X main.version=${VERSION/v}" github.com/AndrewLester/ntpal/cmd/ntpal

site:
	go build -o bin/site -ldflags "-X main.version=${VERSION/v}" github.com/AndrewLester/ntpal/cmd/site

clean:
	rm -rf bin
