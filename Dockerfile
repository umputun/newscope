FROM ghcr.io/umputun/baseimage/buildgo:latest as build

WORKDIR /build
ADD . /build

# build version based on git revision or CI info
RUN \
    if [ -z "$CI" ] ; then \
    echo "runs outside of CI" && version=$(git rev-parse --abbrev-ref HEAD)-$(git log -1 --format=%h)-$(date +%Y%m%d-%H:%M:%S); \
    else version=$GITHUB_REF_NAME-$GITHUB_SHA-$(date +%Y%m%d-%H:%M:%S); fi && \
    echo "version=$version" && \
    go build -mod=vendor -o newscope -ldflags "-X main.version=$version -s -w" ./cmd/newscope


FROM ghcr.io/umputun/baseimage/app:latest

LABEL org.opencontainers.image.source="https://github.com/umputun/newscope"

WORKDIR /srv
COPY --from=build /build/newscope /srv/newscope

RUN chown -R app:app /srv

# newscope runs directly without init system
ENTRYPOINT ["/srv/newscope"]