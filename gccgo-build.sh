#!/bin/bash

export WORK=$GOPATH/src

# gccgo does not by default use SSE4.2 intrinsics for hash/crc32. This
# substitutes in a package that uses them via calls into C, and runs
# slightly different build steps to compile and link in the C.
perl -pi -e 's{hash/crc32}{github.com/twotwotwo/histzip/crc32gcc}' $WORK/github.com/twotwotwo/histzip/histzip.go

find $WORK -name \*.o | xargs --no-run-if-empty rm
find $WORK -name \*.a | xargs --no-run-if-empty rm

mkdir -p $WORK/github.com/twotwotwo/histzip/crc32gcc/_obj/
mkdir -p $WORK/github.com/twotwotwo/histzip/crc32gcc/_obj/exe/
pushd /home/randall/gocode/src/github.com/twotwotwo/histzip/crc32gcc
gccgo -I $WORK -c -g -m64 -fgo-pkgpath=github.com/twotwotwo/histzip/crc32gcc -fgo-relative-import-path=_/home/randall/gocode/src/github.com/twotwotwo/histzip/crc32gcc -o $WORK/github.com/twotwotwo/histzip/crc32gcc/_obj/crc32.o -O3 ./crc32.go
gcc -m64 -O3 -mcrc32 -c -o $WORK/github.com/twotwotwo/histzip/crc32gcc/crc32_c.o $WORK/github.com/twotwotwo/histzip/crc32gcc/crc32.c
ar cru $WORK/github.com/twotwotwo/histzip/libcrc32gcc.a $WORK/github.com/twotwotwo/histzip/crc32gcc/_obj/crc32.o $WORK/github.com/twotwotwo/histzip/crc32gcc/crc32_c.o
popd

mkdir -p $WORK/github.com/twotwotwo/histzip/lrcompress/_obj/
mkdir -p $WORK/github.com/twotwotwo/histzip/lrcompress/_obj/exe/
pushd /home/randall/gocode/src/github.com/twotwotwo/histzip/lrcompress
gccgo -I $WORK -c -g -m64 -fgo-pkgpath=github.com/twotwotwo/histzip/lrcompress -fgo-relative-import-path=_/home/randall/gocode/src/github.com/twotwotwo/histzip/lrcompress -o $WORK/github.com/twotwotwo/histzip/lrcompress/_obj/lrcompress.o -O3 ./lrcompress.go
ar cru $WORK/github.com/twotwotwo/histzip/liblrcompress.a $WORK/github.com/twotwotwo/histzip/lrcompress/_obj/lrcompress.o
popd

mkdir -p $WORK/github.com/twotwotwo/histzip/_obj/
mkdir -p $WORK/github.com/twotwotwo/histzip/_obj/exe/
pushd /home/randall/gocode/src/github.com/twotwotwo/histzip
gccgo -I $WORK -I /home/randall/gocode/pkg/gccgo_linux_amd64 -c -g -m64 -fgo-relative-import-path=_/home/randall/gocode/src/github.com/twotwotwo/histzip -o $WORK/github.com/twotwotwo/histzip/_obj/main.o -O3 ./histzip.go
ar cru $WORK/github.com/twotwotwo/libhistzip.a $WORK/github.com/twotwotwo/histzip/_obj/main.o
cd .
gccgo -o $WORK/github.com/twotwotwo/histzip/_obj/exe/a.out $WORK/github.com/twotwotwo/histzip/_obj/main.o $WORK/github.com/twotwotwo/histzip/lrcompress/_obj/lrcompress.o -m64 -static -Wl,-u,pthread_create -O3 $WORK/github.com/twotwotwo/histzip/libcrc32gcc.a
cp $WORK/github.com/twotwotwo/histzip/_obj/exe/a.out histzip
popd

perl -pi -e 's{github.com/twotwotwo/histzip/crc32gcc}{hash/crc32}' $WORK/github.com/twotwotwo/histzip/histzip.go

cp $WORK/github.com/twotwotwo/histzip/histzip .