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

histzip exists to try to provide good performance packing flat files
containing change histories, where delta coding or other long-range
compression isn't already baked into the format.  If you grab a chunk of
English Wikipedia's change history XML dump (like any pages-meta-history .7z
file from [Wikimedia's December 2013 dump][7]), then pipe the uncompressed
text through histzip and bzip2, the histzip|bzip2 can handle input at on the
order of 100 MB/CPU-second.  That means it runs many times faster for this
particular use case than the general-purpose compression tools used now,
while getting compression ratios near 7zip's on average.  Because
Wikipedia's full change history is several terabytes and dumped monthly,
this might be useful for producing those dumps more quickly.  (To be clear,
I'm not associated with Wikimedia, and nor is this project, though I hope it
can be useful.)

[7]: http://dumps.wikimedia.org/enwiki/20131202/

While compressing, histzip decompresses its output and makes sure it matches
the input by comparing CRCs.  If it should fail to match, you'll see "Can't
continue: test decompression checksum mismatch"; then [you are having a bad
problem and you will not go to space today][8] and should contact me so we
can figure out what's up.  It has happily crunched the full English
Wikipedia change history and some synthetic tests, but anything can happen. 

[8]: http://xkcd.com/1133/

On recent-ish Intel chips in 64-bit mode, [SSE4.2 makes it faster to compute
those CRCs][9], which contributes to the 64-bit builds' better speeds.  You
can build histzip as pure Go for any of the supported platforms with go build, and it will use
the hardware CRC instruction where possible (yay Go standard libraries!). 
To get gccgo linux/amd64 build using the hardware CRC, I wrote some extra
code and did some extra build steps which are recorded in in gccgo-build.sh
and crc32gcc/.

[9]: http://en.wikipedia.org/wiki/SSE4#SSE4.2

Public domain, Randall Farmer, 2013-4.
