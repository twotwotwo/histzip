histzip
=======

Quickly compress wiki change dumps and files like them. More precisely,
compresses out long repeats in files (think 100+ byte repeated stretches)
that may occur relatively long distances apart (think up to a few MB). 
There are builds for [Linux amd64][1], [Linux x86][3], [Windows 64-bit][4]
and [32-bit][5], and [Mac 64-bit][6].  For faster compression on systems
that can run it, a [gccgo Linux amd64][2] build is available, too.

[1]: http://www.rfarmer.net/histzip/histzip.6g
[2]: http://www.rfarmer.net/histzip/histzip
[3]: http://www.rfarmer.net/histzip/histzip.linux386
[4]: http://www.rfarmer.net/histzip/histzip64.exe
[5]: http://www.rfarmer.net/histzip/histzip386.exe
[6]: http://www.rfarmer.net/histzip/histzip.mac

Compress by piping in raw text; decompress by piping in histzip's output. 
You'll usually want to pipe through a "normal" (de)compressor as well, so
compression looks like:

> cat revisions.xml | histzip | bzip2 > revisions.xml.hbz

and decompression looks like:

> cat revisions.xml.hbz | bzip2 -dc | histzip > revisions.xml

On an English Wikipedia dump, histzip's throughput was over 200MB/CPU-second, and a histzip|bzip pipeline ran at about 100 MB/CPU-second. Compression ratios for the histzip|bzip pipeline were roughly similar to [7zip]'s, slightly better on some chunks of history and slightly worse on others.

There are other long-range compressors that might interest you. [rzip] is awesome, and directly inspired some aspects of histzip's implementation. [bm] is a [Bentley-McIlroy][bmpaper] library by CloudFlare also written in Go, compressing matches against a fixed dictionary. [Git][gitdiff], [xdelta], and [open-vcdiff] have other open-source binary diff implementations.

[rzip]: http://rzip.samba.org/
[bm]: https://github.com/cloudflare/bm
[bmpaper]: http://citeseerx.ist.psu.edu/viewdoc/download?doi=10.1.1.11.8470&rep=rep1&type=pdf
[7]: http://dumps.wikimedia.org/enwiki/20131202/
[7zip]: http://www.7-zip.org/sdk.html
[gitdiff]: https://github.com/git/git/blob/master/diff-delta.c
[xdelta]: http://xdelta.org/
[open-vcdiff]: https://code.google.com/p/open-vcdiff/

While compressing, histzip decompresses its output and makes sure it matches
the input by comparing CRCs.  If it should mismatch, you'll see "Can't
continue: test decompression checksum mismatch"; then [you are having a bad
problem and you will not go to space today][8] and should contact me so we
can figure out what's up.  It has happily crunched the full English
Wikipedia change history and some synthetic tests, but anything can happen. 

[8]: http://xkcd.com/1133/

On recent-ish Intel chips in 64-bit mode, [SSE4.2 hardware-accelerates
those CRCs][9], which contributes to the 64-bit builds' better speeds.  You
can build histzip as pure Go for any of the supported platforms with go build, and it will use
the hardware CRC instruction where possible (yay Go standard libraries!). 
To get gccgo linux/amd64 build using the hardware CRC, I wrote some extra
code and did some extra build steps which are recorded in in gccgo-build.sh
and crc32gcc/.

[9]: http://en.wikipedia.org/wiki/SSE4#SSE4.2

Public domain, Randall Farmer, 2013-4.
