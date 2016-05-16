FROM alpine

ADD cachebot_linux_amd64 /

RUN apk --update upgrade && \
    apk add curl ca-certificates && \
    update-ca-certificates && \
    rm -rf /var/cache/apk/*

CMD /cachebot_linux_amd64
