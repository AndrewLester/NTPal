FROM golang:1.19.3

RUN apt-get update && \
    apt-get install -y \
    bash \
    curl \
    make

WORKDIR /app
COPY . /app/
RUN rm -rf .git
RUN go version
RUN make

ENTRYPOINT ["/app/entry.sh"]
