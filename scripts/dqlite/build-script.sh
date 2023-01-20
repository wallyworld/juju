#!/bin/bash
set -ex

# TODO: Make this script idempotent, so that it checks for the
# existence of repositories, requiring only a pull and not a full clone.

# Setup build env
sudo apt-get update
sudo apt-get -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" install \
    gcc automake libtool make gettext autopoint pkg-config tclsh tcl libsqlite3-dev

mkdir build
cd build

# Checkout and build musl. We will use this to avoid depending
# on the hosts libc.
#
# TODO: investigate zig-gcc as an alternative.
wget https://musl.libc.org/releases/musl-1.2.3.tar.gz
tar xf musl-1.2.3.tar.gz
cd musl-1.2.3
./configure
sudo make install

export PATH=${PATH}:/usr/local/musl/bin
export CC=musl-gcc
cd ..

# Setup symlinks so we can access additional headers that 
# don't ship with musl but are needed for our builds
sudo ln -s /usr/include/$(uname -m)-linux-gnu/asm /usr/local/musl/include/asm
sudo ln -s /usr/include/asm-generic /usr/local/musl/include/asm-generic
sudo ln -s /usr/include/linux /usr/local/musl/include/linux

# Grab the queue.h file that does not ship with musl
sudo wget https://dev.midipix.org/compat/musl-compat/raw/main/f/include/sys/queue.h -O /usr/local/musl/include/sys/queue.h

# Install compile dependencies for statically linking everything:
# --------------------------------------------------------------
# libtirpc (required by libnsl)
# libnsl (required by dqlite)
# libuv (required by raft)
# liblz4 (required by raft)
# raft (required by dqlite)
# sqlite3 (required by dqlite)
# dqlite

# libtirpc
git clone https://salsa.debian.org/debian/libtirpc.git --depth 1 --branch ${TAG_LIBTIRPC}
cd libtirpc
chmod +x autogen.sh
./autogen.sh
./configure --disable-shared --disable-gssapi
make
cd ../

# libnsl
git clone https://github.com/thkukuk/libnsl --depth 1 --branch ${TAG_LIBNSL}
cd libnsl
./autogen.sh
autoreconf -i
autoconf
CFLAGS="-I${PWD}/../libtirpc/tirpc" \
        LDFLAGS="-L${PWD}/../libtirpc/src" \
        TIRPC_CFLAGS="-I${PWD}/../libtirpc/tirpc" \
        TIRPC_LIBS="-L${PWD}/../libtirpc/src" \
        ./configure --disable-shared
make
cd ../

# libuv
git clone https://github.com/libuv/libuv.git --depth 1 --branch ${TAG_LIBUV}
cd libuv
./autogen.sh
./configure # we need the .so files as well; see note below
make
cd ../

# liblz4
git clone https://github.com/lz4/lz4.git --depth 1 --branch ${TAG_LIBLZ4}
cd lz4
make lib
cd ../

# raft
git clone https://github.com/canonical/raft.git --depth 1 --branch ${TAG_RAFT}
cd raft
autoreconf -i
CFLAGS="-I${PWD}/../libuv/include -I${PWD}/../lz4/lib" \
        LDFLAGS="-L${PWD}/../libuv/.libs -L${PWD}/../lz4/lib" \
        UV_CFLAGS="-I${PWD}/../libuv/include" \
        UV_LIBS="-L${PWD}/../libuv/.libs" \
        LZ4_CFLAGS="-I${PWD}/../lz4/lib" \
        LZ4_LIBS="-L${PWD}/../lz4/lib" \
        ./configure --disable-shared
make
cd ../

# sqlite3
git clone https://github.com/sqlite/sqlite.git --depth 1 --branch ${TAG_SQLITE}
cd sqlite
./configure --disable-shared
make
cd ../

# dqlite
git clone https://github.com/canonical/dqlite.git --depth 1 --branch ${TAG_DQLITE}
cd dqlite
autoreconf -i
CFLAGS="-I${PWD}/../raft/include -I${PWD}/../sqlite -I${PWD}/../libuv/include -I${PWD}/../lz4/lib -I/usr/local/musl/include -Werror=implicit-function-declaration" \
        LDFLAGS="-L${PWD}/../raft/.libs -L${PWD}/../libuv/.libs -L${PWD}/../lz4/lib -L${PWD}/../libnsl/src" \
        RAFT_CFLAGS="-I${PWD}/../raft/include" \
        RAFT_LIBS="-L${PWD}/../raft/.libs" \
        UV_CFLAGS="-I${PWD}/../libuv/include" \
        UV_LIBS="-L${PWD}/../libuv/.libs" \
        SQLITE_CFLAGS="-I${PWD}/../sqlite" \
        ./configure --disable-shared
make
cd ../

rm -Rf juju-dqlite-static-lib-deps
mkdir juju-dqlite-static-lib-deps

# Collect .a files
# NOTE: for some strange reason we *also* require the libuv and
# liblz4 .so files for the final juju link step even though the
# resulting artifact is statically linked.
cp libuv/.libs/* juju-dqlite-static-lib-deps/
cp lz4/lib/*.a juju-dqlite-static-lib-deps/
cp lz4/lib/*.so* juju-dqlite-static-lib-deps/
cp raft/.libs/*.a juju-dqlite-static-lib-deps/
cp sqlite/.libs/*.a juju-dqlite-static-lib-deps/
cp dqlite/.libs/*.a juju-dqlite-static-lib-deps/

# Collect required headers
mkdir juju-dqlite-static-lib-deps/include
cp -r raft/include/* juju-dqlite-static-lib-deps/include
cp -r sqlite/*.h juju-dqlite-static-lib-deps/include
cp -r dqlite/include/* juju-dqlite-static-lib-deps/include

tar cjvf juju-dqlite-static-lib-deps.tar.bz2 juju-dqlite-static-lib-deps
