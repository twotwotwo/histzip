package lrcompress

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"hash"
	"io"
)

type bufIOLike interface {
	io.ByteReader
	io.Reader
}

// Decompressor copies decompressed content to a Writer.
type Decompressor struct {
	pos    int64     // count of bytes ever written
	mask   int64     // &mask turns pos into a Decompressor offset
	w      io.Writer // output of writes/copies goes here as well as Decompressor
	ring   []byte    // the bytes
	br     bufIOLike
	cksum  hash.Hash
	sumBuf []byte
	sumIn  []byte
	concat bool // reading one block or any number of concatenated ones?
	io.Reader
}

// Makes decompressor. sizeBits is the log2 of the history buffer size (default
// 22 for histzip). h is an optional but recommended checksum function like those in
// hash/crc32. concat is a flag saying whether to read until encountering an empty
// block (if true) or return io.EOF after one block (false; then you can call
// StartRead() to read another block if you wish).
func NewDecompressor(r io.Reader, sizeBits uint, h hash.Hash, concat bool) *Decompressor {
	// Caller ought to be able to read bytes in between blocks for its own nefarious
	// purposes, so I should leave existing ByteReaders alone. Else NewReader
	br, ok := r.(bufIOLike)
	if !ok {
		br = bufio.NewReader(r)
	}
	if h == nil {
		h = noChecksum{}
	}
	return &Decompressor{
		br:     br,
		pos:    0,
		mask:   1<<sizeBits - 1,
		ring:   make([]byte, 1<<sizeBits),
		cksum:  h,
		sumIn:  make([]byte, h.Size()),
		concat: concat,
	}
}

// Clear state for reuse.
func (d *Decompressor) Reset() {
	d.pos = 0
	d.cksum.Reset()
	d.Reader = nil
}

// Load dictionary content. Compressor and decompressor must load byte-identical
// content at the same time, of course.
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

// Copy old content to the current position. If the copy source overlaps the
// destination, will produce repeats.
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

// Decompress a block from rd to w in one shot, retaining state at end.
func (d *Decompressor) copyBlk(w io.Writer) (blkLen int64, err error) {
	cursor := d.pos
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

// Copy somewhere until you hit an empty block (like histzip does)
func (d *Decompressor) copyUntilEmpty(w io.Writer) (written int64, err error) {
	for err == nil {
		var n int64
		n, err = d.copyBlk(w)
		if n == 0 && err != nil {
			return written, nil
		}
		written += n
	}
	return
}

// More efficient alternative to Read when appropriate.
func (d *Decompressor) WriteTo(w io.Writer) (written int64, err error) {
	if d.concat {
		written, err = d.copyUntilEmpty(w)
	} else {
		written, err = d.copyBlk(w)
	}
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return
}

// Read decompressed content. If concat is false, this will return io.EOF at the end
// of a block, and you can then call StartRead() to start on the next block. Reads
// will often less than fill the buffer, and using io.Copy or WriteTo is usually 
// more efficient.
func (d *Decompressor) Read(p []byte) (n int, err error) {
	if d.Reader == nil {
		d.StartRead()
	}
	return d.Reader.Read(p)
}

// See Read(): if concat was set to false in NewDecompressor, call this to start on
// the next block.
func (d *Decompressor) StartRead() {
	pr, pw := io.Pipe()
	go func() {
		_, err := d.WriteTo(pw)
		if err == nil {
			err = io.EOF
		}
		pw.CloseWithError(err)
	}()
	d.Reader = pr
}
