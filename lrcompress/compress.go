package lrcompress

import (
	"encoding/binary"
	"hash"
	"io"
)

// ring buffer (yep, size baked in)
const CompHistBits = 22           // log2 bytes of history for compression
const rMask = 1<<CompHistBits - 1 // &rMask turns offset into ring pos

// compression hashtable
const hBits = 18           // log2 hashtable size
const hMask = 1<<hBits - 1 // c.hTbl[h>>hShift&hMask] is current hashtable entry
const hShift = 32 - hBits
const fMask = 1<<fBits - 1             // hit hashtable if fBits are 1111...
const fBits = CompHistBits - hBits + 1 // 1/2 fill the table

// output format choices
const window = 64          // bytes that must overlap to match
const maxLiteral = 1 << 16 // we'll write this size literal
const maxMatch = 1 << 18   // max match we output (we'll read 1<<histBits)

type compRing [1 << CompHistBits]byte
type compHtbl [1 << hBits]int64

// Compressor is a Writer into which you can dump content.
type Compressor struct {
	pos        int64     // count of bytes ever written
	ring       compRing  // the bytes
	h          uint32    // current rolling hash
	matchPos   int64     // current match start or 0
	matchLen   int64     // current match length or 0
	minMatch   int64     // lowest possible matchPos
	cursor     int64     // "expected" match start
	w          io.Writer // compressed output
	literalLen int64     // current literal length or 0
	encodeBuf  [16]byte  // for varints
	hTbl       compHtbl  // hashtable holding offsets into source file
	cksum      hash.Hash
	sumBuf     []byte
}

// Make a compressor with 1<<CompHistBits of memory, writing output to w.
func NewCompressor(w io.Writer, h hash.Hash) *Compressor {
	return &Compressor{w: w, minMatch: 1, pos: 1, cursor: 1, cksum: h}
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

// found a potential match; see if it checks out and use it if so
func (c *Compressor) tryMatch(ring *compRing, pos, literalLen, minMatch, match int64) (matchLen_ int64, err error) {
	matchPos, matchLen := match, int64(1) // 1 because cur. byte matched
	min := pos - rMask + maxLiteral
	if min < minMatch {
		min = minMatch
	}
	// extend backwards
	for literalLen > 0 &&
		matchPos-1 > min &&
		ring[(pos-matchLen)&rMask] == ring[(matchPos-1)&rMask] {
		literalLen--
		matchPos--
		matchLen++
	}
	if matchLen >= window { // long enough match, flush literal and use it
		// this literal ends before pos-matchLen+1, not pos
		if err = c.putLiteral(pos-matchLen+1, literalLen); err != nil {
			return
		}
		return matchLen, error(nil)
	} else { // short match, ignore
		return 0, error(nil)
	}
}

func (c *Compressor) Write(p []byte) (n int, err error) {
	h, ring, hTbl, pos, matchPos, matchLen, literalLen, minMatch := c.h, &c.ring, &c.hTbl, c.pos, c.matchPos, c.matchLen, c.literalLen, c.minMatch
	c.cksum.Write(p)
	for _, b := range p {
		// can use any 32-bit const with least sig. bits=10b and some higher
		// bits set; even *=6 eventually mixes lower bits into the top ones
		h *= ((0x703a03ac|1)*2)&(1<<32-1) | 1<<31
		h ^= uint32(b)
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
			match := hTbl[h>>hShift&hMask]
			// check if it's in usable range and cur. byte matches, then tryMatch
			if match > minMatch && b == ring[match&rMask] && match > pos-rMask+maxLiteral {
				matchLen, err = c.tryMatch(ring, pos, literalLen, minMatch, match)
				if matchLen > 0 {
					literalLen = 0
					matchPos = match - matchLen + 1
				} else if err != nil {
					return
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

// Clear packer state. Flush first.
func (c *Compressor) Reset() {
	c.minMatch, c.cursor = c.pos, c.pos
	c.matchPos, c.matchLen, c.literalLen = 0, 0, 0
}

// Loads dict content. Does not update checksum. Call only after init or Reset.
func (c *Compressor) Load(p []byte) {
	h, ring, hTbl, pos := c.h, &c.ring, &c.hTbl, c.pos
	c.cksum.Write(p)
	for _, b := range p {
		// can use any 32-bit const with least sig. bits=10b and some higher
		// bits set; even *=6 eventually mixes lower bits into the top ones
		h *= ((0x703a03ac|1)*2)&(1<<32-1) | 1<<31
		h ^= uint32(b)

		// update hashtable and ring
		ring[pos&rMask] = b
		if h&fMask == fMask {
			hTbl[h>>hShift&hMask] = pos
		}
		pos++
	}
	c.h, c.pos = h, pos
	return
}

// Flush any pending match/literal
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

// Flush and write \0 end-of-block marker and checksum, and clear checksum state.
func (c *Compressor) EndBlock() (err error) {
	c.Flush()
	c.sumBuf = c.cksum.Sum(c.sumBuf[:0])
	if err = c.putInt(0); err != nil {
		return
	} else if _, err = c.w.Write(c.sumBuf); err != nil {
		return
	}
	c.cksum.Reset()
	return
}

func (c *Compressor) Close() (err error) {
	return c.EndBlock()
}
