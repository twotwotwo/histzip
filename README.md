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

Compress by piping in raw text; decompress by piping in histzip's output. You typically want to pipe the output through bzip2 as well, so the command line looks like this:

> ./histzip < revisions.xml | bzip2 > revisions.xml.hbz

To decompress:

> bunzip2 < revisions.xml.hbz | ./histzip > revisions.xml

That tends to work fastest and compress best when handling lots of revisions with mostly unchanged content. Running histzip|bzip on the newest/oldest chunks of the English Wikipedia's history, speed ranged from 51 to 151 MB/s, and compression ratios were roughly similar to [7zip]'s, between 8% worse and 10% better.

While compressing, histzip decompresses its output and compares checksums as a self-check.  

There are write-ups attempting to describe [the framing format][framing] and [the format for compressed 
data][lrcompress-format]. You can use the same compression engine in other programs via the histzip/lrcompress library.

[8]: http://xkcd.com/1133/
[framing]: format.md
[lrcompress-format]: lrcompress/format.md

If you're interested in long-range compression, there are other projects that might interest you. [rzip] is awesome, and directly inspired some aspects of histzip's implementation. [bm] is a [Bentley-McIlroy][bmpaper] library by CloudFlare also written in Go, compressing matches against a fixed dictionary (in essence, performing a binary diff). [Git][gitdiff], [xdelta], and [open-vcdiff] also each have open-source binary diff implementations.

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
