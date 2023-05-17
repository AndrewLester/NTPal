FROM golang

RUN apt-get update && \
    apt-get install -y \
    bash \
    curl \
    make

WORKDIR /app
COPY . /app/
RUN rm -rf .git
RUN make

ENTRYPOINT ["/app/entry.sh"]
