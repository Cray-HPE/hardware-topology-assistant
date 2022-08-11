# MIT License
#
# (C) Copyright 2022 Hewlett Packard Enterprise Development LP
#
# Permission is hereby granted, free of charge, to any person obtaining a
# copy of this software and associated documentation files (the "Software"),
# to deal in the Software without restriction, including without limitation
# the rights to use, copy, modify, merge, publish, distribute, sublicense,
# and/or sell copies of the Software, and to permit persons to whom the
# Software is furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included
# in all copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
# THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
# OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
# ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
# OTHER DEALINGS IN THE SOFTWARE.
#

#
# Builder image
#
FROM artifactory.algol60.net/docker.io/library/golang:1.17-alpine AS builder

# Copy all the necessary files to the image.
COPY cmd        $GOPATH/src/github.com/Cray-HPE/hardware-topology-assistant/cmd
COPY internal   $GOPATH/src/github.com/Cray-HPE/hardware-topology-assistant/internal
COPY pkg        $GOPATH/src/github.com/Cray-HPE/hardware-topology-assistant/pkg
COPY main.go    $GOPATH/src/github.com/Cray-HPE/hardware-topology-assistant/main.go
COPY vendor     $GOPATH/src/github.com/Cray-HPE/hardware-topology-assistant/vendor

RUN set -ex \
    && go env -w GO111MODULE=auto \
    && go build -v -o /usr/local/bin/hardware-topology-assistant github.com/Cray-HPE/hardware-topology-assistant


#
# Final image
#
FROM artifactory.algol60.net/csm-docker/stable/docker.io/library/alpine:3.16
LABEL maintainer="Hewlett Packard Enterprise"
STOPSIGNAL SIGTERM

COPY --from=builder /usr/local/bin/hardware-topology-assistant /usr/local/bin/hardware-topology-assistant

ENTRYPOINT [ "hardware-topology-assistant" ]