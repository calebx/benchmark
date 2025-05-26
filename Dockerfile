FROM ubuntu:24.04
COPY ./build/server /server
ENTRYPOINT ["/server"]

