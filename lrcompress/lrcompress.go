package lrcompress

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
)

type compRing [1 << CompHistBits]byte
type compHtbl [1 << hBits]int64

// Compressor is a Writer into which you can dump content.
type Compressor struct {
	pos        int64     // count of bytes ever written
	ring       compRing  // the bytes
	h          uint64    // current rolling hash
	matchPos   int64     // current match start or 0
	matchLen   int64     // current match length or 0
	cursor     int64     // "expected" match start
	w          io.Writer // compressed output
	literalLen int64     // current literal length or 0
	encodeBuf  [16]byte  // for varints
	hTbl       compHtbl  // hashtable holding offsets into source file
}

const CompHistBits = 22    // log2 bytes of history for compression
const hBits = 18           // log2 hashtable size
const window = 64          // bytes that must overlap to match
const maxLiteral = 1 << 16 // we'll write this size literal
const maxMatch = 1 << 18   // max match we output (we'll read larger)
// note ring size during *compression* is baked in at compile time
const rMask = 1<<CompHistBits - 1 // &rMask turns offset into ring pos
// c.hTbl[h>>hShift&hMask] is current hashtable entry
const hMask = 1<<hBits - 1
const hShift = 64 - hBits

// only access hTbl if (h&fMask == fMask)
const fBits uint = CompHistBits - hBits + 1 // 1/2 fill the table
const fMask uint64 = 1<<fBits - 1

// Make a compressor with 1<<CompHistBits of memory, writing output to w.
func NewCompressor(w io.Writer) *Compressor {
	return &Compressor{
		w: w,
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
		if matchLen > 0 {
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
					matchPos-1 > 0 &&
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
		if matchLen == 0 {
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
	return len(p), nil
}

func (c *Compressor) Flush() (err error) {
	if c.matchLen > 0 {
		err = c.putMatch(c.matchPos, c.matchLen)
		c.matchPos, c.matchLen = 0, 0
	} else {
		err = c.putLiteral(c.pos, c.literalLen)
		c.literalLen = 0
	}
	return
}

func (c *Compressor) Close() (err error) {
	if err = c.Flush(); err != nil {
		return
	}
	return c.putInt(0)
}

const maxReadLiteral = 1 << 16

// ring is a ring-buffer of bytes with Copy and Write operations. Writes and
// copies are teed to an io.Writer provided on initialization.
type ring struct {
	pos  int64     // count of bytes ever written
	mask int64     // &mask turns pos into a ring offset
	w    io.Writer // output of writes/copies goes here as well as ring
	ring []byte    // the bytes
}

func newRing(sizeBits uint, w io.Writer) ring {
	return ring{
		pos:  0,
		mask: 1<<sizeBits - 1,
		w:    w,
		ring: make([]byte, 1<<sizeBits),
	}
}

func (r *ring) Write(p []byte) (n int, err error) {
	n, err = r.w.Write(p)
	if err != nil {
		return
	}
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

// ring.Copy copies old content to the current position. If the copy source
// overlaps the destination, Copy will produce repeats.
func (r *ring) Copy(start int64, n int) (err error) {
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
func Decompress(historyBits uint, rd io.Reader, w io.Writer) error {
	br := bufio.NewReader(rd)
	r := newRing(historyBits, w)
	cursor := int64(0)
	literal := [maxReadLiteral]byte{}
	for {
		instr, err := binary.ReadVarint(br)
		if err != nil {
			return err
		}
		if instr > 0 { // copy!
			l := instr
			cursorMove, err := binary.ReadVarint(br)
			if err != nil {
				return err
			}
			cursor += cursorMove
			if err = r.Copy(cursor, int(l)); err != nil {
				return err
			}
			cursor += l
		}
		if instr == 0 { // end of stream!
			return io.EOF
		}
		if instr < 0 { // literal!
			l := -instr
			cursor += l
			if _, err := io.ReadFull(br, literal[:l]); err != nil {
				return err
			}
			if _, err = r.Write(literal[:l]); err != nil {
				return err
			}
		}
	}
}

func NewDecompressor(historyBits uint, rd io.Reader) io.Reader {
	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(Decompress(historyBits, rd, pw))
	}()
	return pr
}
