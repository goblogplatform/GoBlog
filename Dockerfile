# Start from a Debian image with the latest version of Go installed
# and a workspace (GOPATH) configured at /go.
FROM golang:1.26.1

# Copy the local package files to the container's workspace.
ADD . /go/src/github.com/compscidr/goblog/

# Build the outyet command inside the container.
# (You may fetch or manage dependencies here,
# either manually or with a tool like "godep".)
ARG VERSION
RUN cd /go/src/github.com/compscidr/goblog/ && go build -ldflags="-X 'main.Version=$VERSION'" -v .

# Run the outyet command by default when the container starts.
# So `docker exec <container> ./goblog reset-admin-password` finds .env.
WORKDIR /go/src/github.com/compscidr/goblog

ENTRYPOINT cd /go/src/github.com/compscidr/goblog && ./goblog

# Document that the service listens on port 7000.
EXPOSE 7000
