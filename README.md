histzip
=======

Compress wiki change histories, etc. Made for any input with long duplicated sections (100+ bytes) up to a few MB apart.

**Download binaries** for [Linux amd64][1], [Linux x86][3], [Windows 64-bit][4]
and [32-bit][5], and [Mac 64-bit][6].  For faster compression try the [gccgo Linux amd64][2] build.

[1]: http://www.rfarmer.net/histzip/histzip.6g
[2]: http://www.rfarmer.net/histzip/histzip
[3]: http://www.rfarmer.net/histzip/histzip.linux386
[4]: http://www.rfarmer.net/histzip/histzip64.exe
[5]: http://www.rfarmer.net/histzip/histzip386.exe
[6]: http://www.rfarmer.net/histzip/histzip.mac

Compress by piping text through histzip and bzip2 or similar:

> ./histzip < revisions.xml | bzip2 > revisions.xml.hbz

Turn that around to decompress:

> bunzip2 < revisions.xml.hbz | ./histzip > revisions.xml

Running on dumps of English Wikipedia's history, that pipeline ran at 51 MB/s for the newest chunk and 151 MB/s for the oldest. Compression ratios were comparable to [7zip]'s: 8% worse for the new chunk and 10% better for the old chunk.

While compressing, histzip decompresses its output and compares checksums as a self-check.  There are write-ups of [the framing format][framing] and [the format for compressed data][lrcompress-format]. You can use the same compression engine in other programs via the histzip/lrcompress library.

[8]: http://xkcd.com/1133/
[framing]: format.md
[lrcompress-format]: lrcompress/format.md

If you're interested in long-range compression, some other projects might interest you. [rzip] is awesome; histzip lifts some implementation tricks directly from it. [bm] is a [Bentley-McIlroy][bmpaper] library by CloudFlare also written in Go, compressing matches against a fixed dictionary (in essence, performing a binary diff). [Git][gitdiff], [xdelta], and [open-vcdiff] also each have open-source binary diff implementations.

[rzip]: http://rzip.samba.org/
[bm]: https://github.com/cloudflare/bm
[bmpaper]: http://citeseerx.ist.psu.edu/viewdoc/download?doi=10.1.1.11.8470&rep=rep1&type=pdf
[7]: http://dumps.wikimedia.org/enwiki/20131202/
[7zip]: http://www.7-zip.org/sdk.html
[gitdiff]: https://github.com/git/git/blob/master/diff-delta.c
[xdelta]: http://xdelta.org/
[open-vcdiff]: https://code.google.com/p/open-vcdiff/

If you have any trouble or questions, get in touch!

Public domain, Randall Farmer, 2013-4.
