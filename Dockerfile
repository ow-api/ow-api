FROM golang:alpine AS builder

ADD . /build

RUN cd /build && go build -o ow-api

FROM golang:alpine

COPY --from=builder /build/ow-api /usr/bin/ow-api

CMD [ "/usr/bin/ow-api" ]