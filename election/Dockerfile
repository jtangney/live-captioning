FROM golang:1.13 as build-env

WORKDIR /go/src/app
ADD . /go/src/app

RUN go get -d -v ./...
RUN go build -o /go/bin/app

# next stage 
FROM gcr.io/distroless/base
COPY --from=build-env /go/bin/app /
ENTRYPOINT ["/app"]