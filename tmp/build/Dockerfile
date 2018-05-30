FROM alpine:3.6

RUN adduser -D kong-operator
USER kong-operator

ADD tmp/_output/bin/kong-operator /usr/local/bin/kong-operator
