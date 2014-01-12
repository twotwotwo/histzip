// Packs files with long (100+-byte) repetitions in a relatively large
// (4MB by default) window. Public domain, Randall Farmer, 2013.
package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"

	"github.com/twotwotwo/histzip/lrcompress"
)

const decompressMaxHistBits = 26 // read files w/up to this
const Sig = "\xAC\x9A\xDC\xF0"   // random
const VerMajor, VerMinor = 0, 1  // VerMajor++ if not back compat

func critical(a ...interface{}) {
	fmt.Fprintln(os.Stderr, append([]interface{}{"histzip failed:"}, a...)...)
	os.Exit(255)
}

func exitWithUsage(reason string) {
	fmt.Fprintln(os.Stderr, "histzip exiting:", reason)
	fmt.Fprintln(os.Stderr, "to compress:   ./histzip < uncompressed.xml | bzip2 > compressed.hbz")
	fmt.Fprintln(os.Stderr, "to decompress: bunzip2 < compressed.hbz | ./histzip > uncompressed.xml")
	os.Exit(255)
}

func crc32c() hash.Hash32 { return crc32.New(crc32.MakeTable(crc32.Castagnoli)) }

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
	crc := crc32c()
	if err != nil {
		exitWithUsage("couldn't read input on stdin (" + err.Error() + ")")
	}
	head := string(headBytes)
	rejectZippedInput(head)
	if head[:4] == Sig {
		bits, vermajor, extra := uint(head[4]), int(head[5]), int(head[7])
		if vermajor > VerMajor {
			critical("file uses a newer version of format; upgrade, please")
		} else if bits > decompressMaxHistBits {
			critical("file would need", 1<<(bits-20), "MB RAM for decompression (if that's OK, recompile with decompHistBits increased)")
		}
		for i := 0; i < extra+8; i++ {
			_, err = br.ReadByte()
			if err != nil {
				critical(err)
			}
		}
		bw := bufio.NewWriter(os.Stdout)
		mw := io.MultiWriter(bw, crc)
		err := lrcompress.Decompress(bits, br, mw)
		if err != io.EOF {
			critical(err)
		} else if err = bw.Flush(); err != nil {
			critical(err)
		}
		ckSum := uint32(0)
		err = binary.Read(br, binary.BigEndian, &ckSum)
		if err != io.EOF { // checksum optional
			if err != nil {
				critical("error reading checksum:", err)
			}
			ok := ckSum == crc.Sum32()
			if !ok {
				critical("checksum mismatch")
			}
		}
	} else {
		header := append([]byte{}, Sig...)
		header = append(header, lrcompress.CompHistBits, VerMajor, VerMinor, 0)
		if _, err := os.Stdout.Write(header); err != nil {
			critical("could not write header")
		}
		// go decompress and checksum
		checkHash, checkErr := uint32(0), make(chan error)
		pr, pw := io.Pipe()
		w := io.MultiWriter(os.Stdout, pw)
		go func() {
			err := error(nil)
			check := crc32c()
			err = lrcompress.Decompress(lrcompress.CompHistBits, pr, check)
			checkHash = check.Sum32()
			go io.Copy(ioutil.Discard, pr)
			checkErr <- err
		}()
		// compress
		bw := bufio.NewWriter(w)
		c := lrcompress.NewCompressor(bw)
		tr := io.TeeReader(br, crc)
		if _, err = io.Copy(c, tr); err != nil {
			critical(err)
		} else if err = c.Close(); err != nil {
			critical(err)
		} else if err = bw.Flush(); err != nil {
			critical(err)
		}
		// write the checksum
		err = binary.Write(os.Stdout, binary.BigEndian, crc.Sum32())
		if err != nil {
			critical("could not write checksum:", err)
		}
		// verify the test decompression worked
		err = <-checkErr
		if err != io.EOF {
			critical("test decompression error:", err)
		} else if crc.Sum32() != checkHash {
			critical("test decompression checksum mismatch")
		}
	}
}
