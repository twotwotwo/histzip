package lrcompress

import (
	"bufio"
	"encoding/binary"
	"errors"
	"hash"
	"io"
)

// ring is a ring-buffer of bytes with Copy and Write operations. Writes and
// copies are teed to an io.Writer provided on initialization.
type ring struct {
	pos   int64     // count of bytes ever written
	mask  int64     // &mask turns pos into a ring offset
	w     io.Writer // output of writes/copies goes here as well as ring
	ring  []byte    // the bytes
	cksum hash.Hash32
}

func newRing(sizeBits uint, w io.Writer) ring {
	return ring{
		pos:   0,
		mask:  1<<sizeBits - 1,
		w:     w,
		ring:  make([]byte, 1<<sizeBits),
		cksum: getChecksum(),
	}
}

func (r *ring) Write(p []byte) (n int, err error) {
	r.cksum.Write(p)
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
	return n, nil
}

// ring.Copy copies old content to the current position. If the copy source
// overlaps the destination, Copy will produce repeats.
func (r *ring) Copy(start int64, n int) (err error) {
	N := n
	for N > 0 && err == nil {
		n = N
		q := int(start & r.mask)
		// lower piece size (n) if needed
		if start >= r.pos {
			return errors.New("copy starts at current/future byte")
		} else if start < 0 || start < r.pos-int64(len(r.ring)) {
			return errors.New("copy starts too far back")
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

var WrongChecksum = errors.New("checksum mismatch")

// Decompress a block from rd to w in one shot. Retains state at end.
func Decompress(historyBits uint, rd io.Reader, w io.Writer) (err error) {
	br := bufio.NewReader(rd)
	r := newRing(historyBits, w)
	cursor := int64(0)
	maxLen := int64(len(r.ring))
	blkLen := int64(0)
	var literalBuf [maxLiteral]byte
	for {
		instr, err := binary.ReadVarint(br)
		if err != nil {
			return err
		}
		if instr > 0 { // copy!
			l := instr
			if l > maxLen {
				return errors.New("copy too long")
			}
			cursorMove, err := binary.ReadVarint(br)
			if err != nil {
				return err
			}
			cursor += cursorMove
			if err = r.Copy(cursor, int(l)); err != nil {
				return err
			}
			cursor += l
			blkLen += l
		}
		if instr == 0 { // end of block!
			if blkLen == 0 {
				return io.EOF
			}
			ckSum := uint32(0)
			if err = binary.Read(br, binary.BigEndian, &ckSum); err != nil {
				return err
			}
			if r.cksum.Sum32() != ckSum {
				return WrongChecksum
			}
			r.cksum.Reset()
		}
		if instr < 0 { // literal!
			l := -instr
			if l > maxLen {
				return errors.New("literal too long")
			}
			cursor += l
			blkLen += l
			for l > 0 {
				chunk := int(l)
				if chunk > maxLiteral {
					chunk = maxLiteral
				}
				n, err := br.Read(literalBuf[:chunk])
				if err == io.EOF && n < chunk {
					return io.ErrUnexpectedEOF
				} else if err != nil {
					return err
				}
				if _, err = r.Write(literalBuf[:n]); err != nil {
					return err
				}
				l -= int64(n)
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
