# stage 0
FROM --platform=$BUILDPLATFORM golang:bullseye AS builder

ARG TARGETPLATFORM

WORKDIR /go/src/github.com/PierreZ/goStatic
COPY . .

RUN mkdir ./bin && \
    apt-get update && apt-get install -y upx && \
    GOOS=$(echo $TARGETPLATFORM | cut -f1 -d/) && \
    GOARCH=$(echo $TARGETPLATFORM | cut -f2 -d/) && \
    GOARM=$(echo $TARGETPLATFORM | cut -f3 -d/ | sed "s/v//" ) && \
    CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} GOARM=${GOARM} go build ${BUILD_ARGS} -ldflags="-s -w" -tags netgo -installsuffix netgo -o ./bin/goStatic && \
    mkdir ./bin/etc && \
    ID=65534 && \
    upx -9 ./bin/goStatic && \
    echo $ID && \
    echo "gostatic:x:$ID:$ID::/sbin/nologin:/bin/false" > ./bin/etc/passwd && \
    echo "gostatic:x:$ID:gostatic" > ./bin/etc/group

FROM scratch
WORKDIR /
COPY --from=builder /go/src/github.com/PierreZ/goStatic/bin/ .
USER gostatic
ENTRYPOINT ["/goStatic"]
 
