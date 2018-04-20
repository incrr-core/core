FROM alpine:3.7
LABEL maintainer "nika jones <njones@incrr.io>"

RUN apk update && apk add ca-certificates && rm -rf /var/cache/apk/*

COPY vendor             /opt/incrr/vendor

ENV INCRR_IMG_BUILD_VER 0.0.1

EXPOSE  80 443
WORKDIR /opt/incrr
ENTRYPOINT [ "./server" ]

# these are the most frequently changing things so they are last
COPY incrr-core-server     /opt/incrr/server