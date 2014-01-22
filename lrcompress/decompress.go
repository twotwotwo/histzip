package lrcompress

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"hash"
	"io"
)

type BufIOLike interface {
	io.ByteReader
	io.Reader
}

// Decompressor is a Decompressor-buffer of bytes with Copy and Write operations. Writes and
// copies are teed to an io.Writer provided on initialization.
type Decompressor struct {
	pos    int64     // count of bytes ever written
	cursor int64     // is same distance back from pos as the last copy was
	mask   int64     // &mask turns pos into a Decompressor offset
	w      io.Writer // output of writes/copies goes here as well as Decompressor
	ring   []byte    // the bytes
	br     BufIOLike
	cksum  hash.Hash
	sumBuf []byte
	sumIn  []byte
}

func NewDecompressor(r io.Reader, sizeBits uint, h hash.Hash) *Decompressor {
	br, ok := r.(BufIOLike)
	if !ok {
		br = bufio.NewReader(r)
	}
	return &Decompressor{
		br:    br,
		pos:   0,
		mask:  1<<sizeBits - 1,
		ring:  make([]byte, 1<<sizeBits),
		cksum: h,
		sumIn: make([]byte, h.Size()),
	}
}

// Load dictionary content.
func (d *Decompressor) Load(p []byte) {
	d.cksum.Write(p)
	for len(p) > 0 {
		l := len(p)
		pos := int(d.pos & d.mask)
		if pos+l > len(d.ring) {
			l = len(d.ring) - pos
		}
		copy(d.ring[pos:pos+l], p[:l])
		p = p[l:]
		d.pos += int64(l)
	}
}

func (d *Decompressor) write(p []byte) (n int, err error) {
	n, err = d.w.Write(p)
	if err != nil {
		return
	}
	d.Load(p)
	return n, nil
}

// Decompressor.Copy copies old content to the current position. If the copy source
// overlaps the destination, Copy will produce repeats.
func (d *Decompressor) copy(start int64, n int) (err error) {
	N := n
	for N > 0 && err == nil {
		n = N
		q := int(start & d.mask)
		// lower piece size (n) if needed
		if start >= d.pos {
			return errors.New("copy starts at current/future byte")
		} else if start < 0 || start < d.pos-int64(len(d.ring)) {
			return errors.New("copy starts too far back")
		} else if start+int64(n) > d.pos { // src overlaps dest
			n = int(d.pos - start)
		}
		if q+n > len(d.ring) { // source wraps around
			n = len(d.ring) - q
		}
		// do the copy and any write
		start += int64(n)
		if _, err := d.write(d.ring[q : q+n]); err != nil {
			return err
		}
		N -= n
	}
	return
}

var WrongChecksum = errors.New("checksum mismatch")

// Decompress a block from rd to w in one shot. Retains state at end.
func (d *Decompressor) CopyBlock(w io.Writer) (blkLen int64, err error) {
	cursor := d.cursor
	br := d.br
	d.w = w
	maxLen := int64(len(d.ring))
	var literalBuf [maxLiteral]byte
	for {
		instr, err := binary.ReadVarint(br)
		if err != nil {
			return blkLen, err
		}
		if instr > 0 { // copy!
			l := instr
			if l > maxLen {
				return blkLen, errors.New("copy too long")
			}
			cursorMove, err := binary.ReadVarint(br)
			if err != nil {
				return blkLen, err
			}
			cursor += cursorMove
			if err = d.copy(cursor, int(l)); err != nil {
				return blkLen, err
			}
			cursor += l
			blkLen += l
		}
		if instr == 0 { // end of block!
			d.sumBuf = d.cksum.Sum(d.sumBuf[:0])
			if _, err = io.ReadFull(br, d.sumIn); err != nil {
				return blkLen, err
			}
			if !bytes.Equal(d.sumBuf, d.sumIn) {
				return blkLen, WrongChecksum
			}
			d.cursor = cursor
			d.cksum.Reset()
			return blkLen, nil
		}
		if instr < 0 { // literal!
			l := -instr
			if l > maxLen {
				return blkLen, errors.New("literal too long")
			}
			cursor += l
			blkLen += l
			for l > 0 {
				chunk := int(l)
				if chunk > maxLiteral {
					chunk = maxLiteral
				}
				_, err := io.ReadFull(br, literalBuf[:chunk])
				if err != nil {
					return blkLen, err
				}
				if _, err = d.write(literalBuf[:chunk]); err != nil {
					return blkLen, err
				}
				l -= int64(chunk)
			}
		}
	}
}

func (d *Decompressor) CopyUntilEOF(w io.Writer) (written int64, err error) {
	for err == nil {
		var n int64
		n, err = d.CopyBlock(w)
		written += n
	}
	return
}

func (d *Decompressor) CopyUntilEmpty(w io.Writer) (written int64, err error) {
	for err == nil {
		var n int64
		n, err = d.CopyBlock(w)
		if n == 0 {
			return written, io.EOF
		}
		written += n
	}
	return
}

// Reader that stops after one block
func (d *Decompressor) BlockReader() io.Reader {
	pr, pw := io.Pipe()
	go func() { _, err := d.CopyBlock(pw); pw.CloseWithError(err) }()
	return pr
}

// Reader that goes until EOF
func (d *Decompressor) UntilEOFReader() io.Reader {
	pr, pw := io.Pipe()
	go func() { _, err := d.CopyUntilEOF(pw); pw.CloseWithError(err) }()
	return pr
}

// Reader that goes until empty block
func (d *Decompressor) UntilEmptyReader() io.Reader {
	pr, pw := io.Pipe()
	go func() { _, err := d.CopyUntilEmpty(pw); pw.CloseWithError(err) }()
	return pr
}
