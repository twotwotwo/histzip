// Packs files with long (100+-byte) repetitions in a relatively large
// (4MB by default) window. Public domain, Randall Farmer, 2013.
package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"

	"github.com/twotwotwo/histzip/lrcompress"
)

const decompressMaxHistBits = 26 // read files w/up to this
const Sig = "\xAC\x9A\xDC\xF0"   // random
const VerMajor, VerMinor = 0, 2  // VerMajor++ if not back compat
const ChunkSize = 1 << 26

func critical(a ...interface{}) {
	fmt.Fprint(os.Stderr, "histzip failed: ")
	fmt.Fprintln(os.Stderr, a...)
	os.Exit(255)
}

func exitWithUsage(reason string) {
	fmt.Fprintln(os.Stderr, "histzip exiting:", reason)
	fmt.Fprintln(os.Stderr, "to compress:   "+os.Args[0]+" < uncompressed.xml | bzip2 > compressed.hbz")
	fmt.Fprintln(os.Stderr, "to decompress: bunzip2 < compressed.hbz | "+os.Args[0]+" > uncompressed.xml")
	os.Exit(255)
}

func rejectZippedInput(header string) {
	badSigs := []string{"BZh", "7z", "\x1F\x8B", "PK", "\xFD7zXZ"}
	for _, sig := range badSigs {
		if strings.HasPrefix(header, sig) {
			exitWithUsage("won't work on compressed data")
		}
	}
}

// main() handles the framing format including checksums, and self-tests
func main() {

	// MAKE SURE WE'RE INVOKED RIGHT AND GET SOME INFO
	if len(os.Args) > 1 {
		exitWithUsage("can't take any files or switches on command line; just pipe in input and redirect to output")
	}
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		exitWithUsage("got interrupt")
	}()
	br := bufio.NewReader(os.Stdin)
	headBytes, err := br.Peek(8)
	if err != nil {
		exitWithUsage("couldn't read input on stdin (" + err.Error() + ")")
	}
	head := string(headBytes)
	rejectZippedInput(head)

	if head[:4] == Sig { // decompress
		bits, vermajor, _, extra := uint(head[4]), int(head[5]), int(head[6]), int(head[7])
		if vermajor > VerMajor {
			critical("file uses a newer version of format; upgrade, please")
		} else if bits > decompressMaxHistBits {
			critical("file would need", 1<<(bits-20), "MB RAM for decompression (if that's OK, recompile with decompHistBits increased)")
		}
		for i := 0; i < extra+8; i++ { // skip extra data
			_, err = br.ReadByte()
			if err != nil {
				critical(err)
			}
		}
		bw := bufio.NewWriter(os.Stdout)
		err := lrcompress.Decompress(bits, br, bw)
		if err != io.EOF {
			critical(err)
		} else if err = bw.Flush(); err != nil {
			critical(err)
		}
	} else { // compress
		// WRITE HEADER
		header := append([]byte{}, Sig...)
		header = append(header, lrcompress.CompHistBits, VerMajor, VerMinor, 0)
		if _, err := os.Stdout.Write(header); err != nil {
			critical("could not write header")
		}

		// go decompress and checksum
		checkErr := make(chan error)
		pr, pw := io.Pipe()
		w := io.MultiWriter(os.Stdout, pw)
		go func() {
			err := lrcompress.Decompress(lrcompress.CompHistBits, pr, ioutil.Discard)
			go io.Copy(ioutil.Discard, pr) // ensure pipe drained even on err
			checkErr <- err
		}()

		// compress
		bw := bufio.NewWriter(w)
		c := lrcompress.NewCompressor(bw)
		for {
			_, err := io.CopyN(c, br, ChunkSize)
			if err != nil { // something special happened
				if err == io.EOF { // end of input
					if err = c.Flush(); err != nil { // finish block
						critical(err)
					} else if err = c.Flush(); err != nil { // write empty block
						critical(err)
					}
					break // we're done
				} else if err != nil { // read/write error, bail out
					critical(err)
				}
			}
			// nothing special happened; just flush
			if err = c.Flush(); err != nil {
				critical(err)
			}
			// look for any test decompress errors mid-stream
			select {
			case err = <-checkErr: // bah; even EOF shouldn't happen yet here, so die
				critical("test decompression error:", err)
			default:
			}
		}
		if err = bw.Flush(); err != nil {
			critical(err)
		}
		pw.Close()
		// verify the test decompression worked
		err = <-checkErr
		if err != io.EOF {
			critical("test decompression error:", err)
		}
	}
}
