FROM golang:1.19.3

RUN apt-get update && \
    apt-get install -y \
    bash \
    curl \
    tmux \
    make

RUN go install github.com/DarthSim/overmind/v2@latest
WORKDIR /app
COPY . /app/
RUN go version
RUN export VERSION=$(git describe --tags $(git rev-list --tags --max-count=1)); make

CMD ["overmind", "start"]
