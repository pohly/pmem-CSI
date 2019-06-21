# Common base image for building PMEM-CSI:
# - up-to-date Clear Linux
# - ndctl installed
FROM clearlinux:base AS build

ARG VERSION="unknown"
ARG NDCTL_VERSION="65"
ARG NDCTL_CONFIGFLAGS="--libdir=/usr/lib64 --disable-docs --without-systemd --without-bash"
ARG NDCTL_BUILD_DEPS="os-core-dev devpkg-util-linux devpkg-kmod devpkg-json-c"

#pull dependencies required for downloading and building libndctl
ARG CACHEBUST
RUN swupd update && swupd bundle-add ${NDCTL_BUILD_DEPS} go-basic-dev && rm -rf /var/lib/swupd
# Workaround for "pkg-config: error while loading shared libraries" when using older Docker
# (see https://github.com/clearlinux/distribution/issues/831)
RUN ldconfig

WORKDIR /
RUN curl --fail --location --remote-name https://github.com/pmem/ndctl/archive/v${NDCTL_VERSION}.tar.gz
RUN tar zxvf v${NDCTL_VERSION}.tar.gz && mv ndctl-${NDCTL_VERSION} ndctl
WORKDIR /ndctl
RUN ./autogen.sh
RUN ./configure ${NDCTL_CONFIGFLAGS}
RUN make install

# Workaround for "error while loading shared libraries: libndctl.so.6" when using older Docker (?)
# and running "make test" inside this container.
# - same as https://github.com/clearlinux/distribution/issues/831?
RUN ldconfig

# Image in which PMEM-CSI binaries get built.
FROM build as binaries

# build pmem-csi-driver
ADD . /go/src/github.com/intel/pmem-csi
ENV GOPATH=/go
ENV PKG_CONFIG_PATH=/usr/lib/pkgconfig/
WORKDIR /go/src/github.com/intel/pmem-csi
ARG BIN_SUFFIX
# Here we choose explicitly which binaries we want in the image and in
# which flavor (production or testing). The actual binary name in the
# image is going to be the same, to avoid unnecessary deployment
# differences.
RUN make VERSION=${VERSION} pmem-csi-driver${BIN_SUFFIX} pmem-vgm${BIN_SUFFIX} pmem-ns-init${BIN_SUFFIX} && \
    mkdir -p /go/bin/ && \
    mv _output/pmem-csi-driver${BIN_SUFFIX} /go/bin/pmem-csi-driver && \
    mv _output/pmem-vgm${BIN_SUFFIX} /go/bin/pmem-vgm && \
    mv _output/pmem-ns-init${BIN_SUFFIX} /go/bin/pmem-ns-init

# Clean image for deploying PMEM-CSI.
FROM clearlinux:base
LABEL maintainers="Intel"
LABEL description="PMEM CSI Driver"

# update and install needed bundles:
# file - driver uses file utility to determine filesystem type
# xfsprogs - XFS filesystem utilities
# storge-utils - for lvm2 and ext4(e2fsprogs) utilities
ARG CACHEBUST
RUN swupd update && swupd bundle-add file xfsprogs storage-utils && rm -rf /var/lib/swupd
# Workaround for "pkg-config: error while loading shared libraries" when using older Docker
# (see https://github.com/clearlinux/distribution/issues/831)
RUN ldconfig

# move required binaries and libraries to clean container
COPY --from=binaries /usr/lib64/libndctl.so.* /usr/lib/
COPY --from=binaries /usr/lib64/libdaxctl.so.* /usr/lib/
RUN mkdir -p /go/bin
COPY --from=binaries /go/bin/ /go/bin/
# default lvm config uses lvmetad and throwing below warning for all lvm tools
# WARNING: Failed to connect to lvmetad. Falling back to device scanning.
# So, ask lvm not to use lvmetad
RUN mkdir -p /etc/lvm
RUN echo "global { use_lvmetad = 0 }" >> /etc/lvm/lvm.conf

ENV LD_LIBRARY_PATH=/usr/lib
ENTRYPOINT ["/go/bin/pmem-csi-driver"]
