FROM golang:1.23.5-alpine AS build

RUN mkdir /build
WORKDIR /build
COPY *.go *.mod *.sum ./

RUN CGO_ENABLED=0 go build --ldflags '-extldflags "-static"'

FROM scratch
COPY --from=build /build/yamprox /bin/yamprox
ENTRYPOINT ["/bin/yamprox"]
