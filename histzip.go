// Packs files with long (100+-byte) repetitions in a relatively large
// (4MB by default) window. Public domain, Randall Farmer, 2013.
package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"io/ioutil"
	"os"
)

var CRCTable = crc32.MakeTable(crc32.Castagnoli)

type compRing [1 << compHistBits]byte
type compHtbl [1 << hBits]int64

// Compressor is a Writer into which you can dump content.
type Compressor struct {
	pos        int64       // count of bytes ever written
	ring       compRing    // the bytes
	h          uint64      // current rolling hash
	CRC        hash.Hash32 // CRC32C f/self-test
	matchPos   int64       // current match start or 0
	matchLen   int64       // current match length or 0
	cursor     int64       // "expected" match start
	w          io.Writer   // compressed output
	literalLen int64       // current literal length or 0
	encodeBuf  [16]byte    // for varints
	hTbl       compHtbl    // hashtable holding offsets into source file
}

const compHistBits = 22    // log2 bytes of history for compression
const hBits = 18           // log2 hashtable size
const window = 64          // bytes that must overlap to match
const maxLiteral = 1 << 16 // we'll write this size literal
const maxMatch = 1 << 18   // max match we output (we'll read larger)
// note ring size during *compression* is baked in at compile time
const rMask = 1<<compHistBits - 1 // &rMask turns offset into ring pos
// c.hTbl[h>>hShift&hMask] is current hashtable entry
const hMask = 1<<hBits - 1
const hShift = 64 - hBits

// only access hTbl if (h&fMask == fMask)
const fBits uint = compHistBits - hBits + 1 // 1/2 fill the table
const fMask uint64 = 1<<fBits - 1

// Make a compressor with 1<<historyBits of memory, writing output to w.
func NewCompressor(w io.Writer) *Compressor {
	return &Compressor{
		w:   w,
		CRC: crc32.New(CRCTable),
	}
}

func (c *Compressor) putInt(i int64) (err error) {
	n := binary.PutVarint(c.encodeBuf[:], i)
	_, err = c.w.Write(c.encodeBuf[:n])
	return
}

func (c *Compressor) putMatch(matchPos, matchLen int64) (err error) {
	err = c.putInt(matchLen)
	if err != nil {
		return
	}
	err = c.putInt(matchPos - c.cursor)
	c.cursor = matchPos + matchLen
	return
}

func (c *Compressor) putLiteral(pos, literalLen int64) (err error) {
	if literalLen == 0 {
		return
	}
	err = c.putInt(-literalLen)
	if err != nil {
		return
	}
	if literalLen > pos&rMask {
		_, err = c.w.Write(c.ring[(pos-literalLen)&rMask:])
		if err != nil {
			return
		}
		_, err = c.w.Write(c.ring[:pos&rMask])
	} else {
		_, err = c.w.Write(c.ring[(pos-literalLen)&rMask : pos&rMask])
	}
	if err != nil {
		return
	}
	c.cursor += literalLen
	return
}

func (c *Compressor) Write(p []byte) (n int, err error) {
	h, ring, hTbl, pos, matchPos, matchLen, literalLen := c.h, &c.ring, &c.hTbl, c.pos, c.matchPos, c.matchLen, c.literalLen
	for _, b := range p {
		// can use any 32-bit const with least sig. bits=10b and some higher
		// bits set; even *=6 eventually mixes lower bits into the top ones
		h *= ((0x703a03ac | 1) * 2) & (1<<32 - 1)
		h ^= uint64(b)
		// if we're in a match, extend or end it
		if matchPos > 0 {
			// try to extend it
			if ring[(matchPos+matchLen)&rMask] == b &&
				matchLen < maxMatch {
				matchLen++
			} else {
				// can't extend it, flush out what we have
				if err = c.putMatch(matchPos, matchLen); err != nil {
					return
				}
				matchPos, matchLen = 0, 0
			}
		} else if literalLen > window && h&fMask == fMask {
			// see if we can *start* a match here.
			// get the hashtable entry
			match := hTbl[h>>hShift&hMask]
			// check if it's in the usable range and cur. byte matches
			if match > 0 && b == ring[match&rMask] && match > pos-rMask+maxLiteral {
				matchPos, matchLen = match, 1 // 1 because cur. byte matched
				// extend backwards
				for literalLen > 0 &&
					matchPos-1 > pos-rMask+maxLiteral &&
					ring[(pos-matchLen)&rMask] == ring[(matchPos-1)&rMask] {
					literalLen--
					matchPos--
					matchLen++
				}
				if matchLen < window { // short match, ignore
					literalLen += matchLen - 1
					matchLen, matchPos = 0, 0
				} else { // match was long enough
					// this literal ends before pos-matchLen+1, not pos
					if err = c.putLiteral(pos-matchLen+1, literalLen); err != nil {
						return
					}
					literalLen = 0
				}
			}
		}
		// (still) not in a match, so just extend the literal
		if matchPos == 0 {
			if literalLen == maxLiteral {
				if err = c.putLiteral(pos, literalLen); err != nil {
					return
				}
				literalLen = 0
			}
			literalLen++
		}

		// update hashtable and ring
		ring[pos&rMask] = b
		if h&fMask == fMask {
			hTbl[h>>hShift&hMask] = pos
		}
		pos++
	}
	c.h, c.pos, c.matchPos, c.matchLen, c.literalLen = h, pos, matchPos, matchLen, literalLen
	_, _ = c.CRC.Write(p)
	return len(p), nil
}

func (c *Compressor) Flush() (err error) {
	if c.matchLen > 0 {
		err = c.putMatch(c.matchPos, c.matchLen)
	} else {
		err = c.putLiteral(c.pos, c.literalLen)
	}
	if err != nil {
		return
	}
	return c.putInt(0)
}

const maxReadLiteral = 1 << 16

// Ring is a ring-buffer of bytes with Copy and Write operations. Writes and
// copies are teed to an io.Writer provided on initialization.
type Ring struct {
	pos  int64       // count of bytes ever written
	mask int64       // &mask turns pos into a ring offset
	CRC  hash.Hash32 // CRC32C for self-check
	w    io.Writer   // output of writes/copies goes here as well as ring
	ring []byte      // the bytes
}

func NewRing(sizeBits uint, w io.Writer) Ring {
	return Ring{
		pos:  0,
		mask: 1<<sizeBits - 1,
		CRC:  crc32.New(CRCTable),
		w:    w,
		ring: make([]byte, 1<<sizeBits),
	}
}

func (r *Ring) Write(p []byte) (n int, err error) {
	n, err = r.w.Write(p)
	if err != nil {
		return
	}
	_, _ = r.CRC.Write(p)
	for len(p) > 0 && err == nil {
		l := len(p)
		pos := int(r.pos & r.mask)
		if pos+l > len(r.ring) {
			l = len(r.ring) - pos
		}
		copy(r.ring[pos:pos+l], p[:l])
		p = p[l:]
		r.pos += int64(l)
	}
	return len(p), nil
}

// Ring.Copy copies old content to the current position. If the copy source
// overlaps the destination, Copy will produce repeats.
func (r *Ring) Copy(start int64, n int) (err error) {
	N := n
	for N > 0 && err == nil {
		n = N
		q := int(start & r.mask)
		// lower piece size (n) if needed
		if start == r.pos { // unsupported but don't hang forever
			return errors.New("zero offset for copy unsupported")
		} else if n > len(r.ring) {
			return errors.New("copies limited to length of hist buffer")
		} else if start+int64(n) > r.pos { // src overlaps dest
			n = int(r.pos - start)
		}
		if q+n > len(r.ring) { // source wraps around
			n = len(r.ring) - q
		}
		// do the copy and any write
		start += int64(n)
		if _, err := r.Write(r.ring[q : q+n]); err != nil {
			return err
		}
		N -= n
	}
	return
}

// Decompress input from rd to w in one shot. Does not handle framing format.
func Decompress(historyBits uint, rd io.Reader, w io.Writer) (err error, H uint32) {
	br := bufio.NewReader(rd)
	r := NewRing(historyBits, w)
	cursor := int64(0)
	literal := [maxReadLiteral]byte{}
	for {
		instr, err := binary.ReadVarint(br)
		if err != nil {
			return err, 0
		}
		if instr > 0 { // copy!
			l := instr
			cursorMove, err := binary.ReadVarint(br)
			if err != nil {
				return err, 0
			}
			cursor += cursorMove
			if err = r.Copy(cursor, int(l)); err != nil {
				return err, 0
			}
			cursor += l
		}
		if instr == 0 { // end of stream!
			return nil, r.CRC.Sum32()
		}
		if instr < 0 { // literal!
			l := -instr
			cursor += l
			if _, err := io.ReadFull(br, literal[:l]); err != nil {
				return err, 0
			}
			if _, err = r.Write(literal[:l]); err != nil {
				return err, 0
			}
		}
	}
}

const decompressMaxHistBits = 26         // read files w/up to this
var Sig = []byte{0xac, 0x9a, 0xdc, 0xf0} // random
const VerMajor, VerMinor = 0, 1          // VerMajor++ if not back compat

func critical(a ...interface{}) {
	fmt.Fprintln(os.Stderr, append([]interface{}{"Can't continue:"}, a...)...)
	os.Exit(255)
}

// main() handles the framing format including checksums, and self-tests
func main() {
	br := bufio.NewReader(os.Stdin)
	head, err := br.Peek(8)
	if err != nil {
		critical("could not read header")
	}
	if bytes.Equal(head[:4], Sig) {
		bits, vermajor, extra := uint(head[4]), int(head[5]), int(head[7])
		if vermajor > VerMajor {
			critical("file uses a new major version of format")
		}
		if bits > decompressMaxHistBits {
			critical("file would need", 1<<(bits-20), "MB RAM for decompression (if that's OK, recompile with decompHistBits increased)")
		}
		for i := 0; i < extra+8; i++ {
			_, err = br.ReadByte()
			if err != nil {
				critical(err)
			}
		}
		bw := bufio.NewWriter(os.Stdout)
		err, H := Decompress(bits, br, bw)
		if err != nil {
			critical(err)
		}
		if err = bw.Flush(); err != nil {
			critical(err)
		}
		ckSum := uint32(0)
		err = binary.Read(br, binary.BigEndian, &ckSum)
		if err != io.EOF { // checksum optional
			if err != nil {
				critical("error reading checksum:", err)
			}
			ok := ckSum == H
			if !ok {
				critical("checksum mismatch")
			}
		}
	} else {
		header := append([]byte{}, Sig...)
		header = append(header, compHistBits, VerMajor, VerMinor, 0)
		if _, err := os.Stdout.Write(header); err != nil {
			critical("could not write header")
		}
		w := io.Writer(os.Stdout)
		// go decompress and checksum
		checkHash, checkErr := uint32(0), make(chan error)
		p, q := io.Pipe()
		w = io.MultiWriter(os.Stdout, q)
		go func() {
			err := error(nil)
			err, checkHash = Decompress(compHistBits, p, ioutil.Discard)
			go io.Copy(ioutil.Discard, p)
			checkErr <- err
		}()
		// compress
		bw := bufio.NewWriter(w)
		c := NewCompressor(bw)
		if _, err = io.Copy(c, br); err != nil {
			critical(err)
		}
		if err = c.Flush(); err != nil {
			critical(err)
		}
		if err = bw.Flush(); err != nil {
			critical(err)
		}
		// verify the test decompression worked
		err = <-checkErr
		if err != nil {
			critical("test decompression error:", err)
		}
		if c.CRC.Sum32() != checkHash {
			critical("test decompression checksum mismatch")
		}
		// write the checksum
		err = binary.Write(os.Stdout, binary.BigEndian, c.CRC.Sum32())
		if err != nil {
			critical("could not write checksum:", err)
		}
	}
}
