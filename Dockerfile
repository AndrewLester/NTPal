FROM golang

RUN apt-get update && \
    apt-get install -y \
    bash \
    curl \
    make

WORKDIR /app
COPY . /app/
RUN ls
RUN echo $GOPATH
RUN make

ENTRYPOINT ["entry.sh"]
