package lrcompress

// test a

import (
	"bytes"
	"crypto/rc4"
	"hash"
	"hash/crc32"
	"io"
	"testing"
)

func crc() hash.Hash { return crc32.New(crc32.MakeTable(crc32.Castagnoli)) }

// Test lrcompress in a diff-like scenario (c.Load(a); c.Write(b); decompress)
func TestDiff(t *testing.T) {
	// a is random
	a := make([]byte, 5000)
	rndSource, err := rc4.NewCipher([]byte("hello"))
	if err != nil {
		t.Error("couldn't set up garbage source")
	}
	rndSource.XORKeyStream(a, a)

	// b is a with a few bytes tweaked
	b := make([]byte, 5000)
	copy(b, a)
	b[10], b[11], b[12] = 0, 0, 0

	// diffBuf holds a diff
	diffBuf := new(bytes.Buffer)
	c := NewCompressor(diffBuf, crc())
	c.Load(a)
	n, err := c.Write(b)
	if err != nil {
		t.Error("Error writing diff:", err)
	} else if n != len(b) {
		t.Error("Got", n, "bytes from write, expected", len(a))
	}
	err = c.Close()
	if err != nil {
		t.Error("Couldn't close diff:", err)
	} else if diffBuf.Len() < 9 || diffBuf.Len() > 50 {
		t.Error("Diff was not in the expected size range")
	}

	// unpacking the diff gets the original
	d := NewDecompressor(diffBuf, 22, crc(), false)
	d.Load(a)

	// In real life you would io.Copy here, but we want to test the Read() method
	// that Copy bypasses (it uses decompressor.WriteTo(dst))
	unpacked := make([]byte, 6000)
	remaining := unpacked
	unpackedCount := 0
	for err == nil {
		n, err = d.Read(remaining)
		remaining = remaining[n:]
		unpackedCount += n
	}
	unpacked = unpacked[:unpackedCount]

	if err != io.EOF {
		t.Error("Couldn't read back diff:", err)
	} else if len(unpacked) != len(b) {
		t.Error("Read back", n, "bytes, expected", len(a))
	} else if !bytes.Equal(b, unpacked) {
		t.Errorf("Expanded diff does not match original")
	}
}

// Verify we don't remember things after Reset
func TestReset(t *testing.T) {
	// a is random
	a := make([]byte, 5000)
	rndSource, err := rc4.NewCipher([]byte("hello"))
	if err != nil {
		t.Error("couldn't set up garbage source")
	}
	rndSource.XORKeyStream(a, a)

	// write a
	diffBuf := new(bytes.Buffer)
	c := NewCompressor(diffBuf, crc())
	_, _ = c.Write(a)

	// reset
	c.Reset()
	diffBuf.Reset()

	// write a again, expect no compression
	_, _ = c.Write(a)
	c.Close()
	if diffBuf.Len() < len(a) {
		t.Error("Should not have found matches after Reset")
	}
}

// Test a long overlapping repeat, potential corner case for unpacker
func TestRepeats(t *testing.T) {
	a := []byte(`abcdefghijklmnopqrstuvwxyz9876543210ABCDEFG`)
	a = append(a, a...) // 2x
	a = append(a, a...) // 4x
	a = append(a, a...) // 8x

	// compress
	buf := new(bytes.Buffer)
	c := NewCompressor(buf, crc())
	_, err := c.Write(a)
	if err != nil {
		t.Error(err)
	} else if err = c.Close(); err != nil {
		t.Error(err)
	}

	// better have compressed
	if buf.Len() > len(a)/6 {
		t.Error("Compressor did not see repeats")
	}

	unpackedBuf := new(bytes.Buffer)
	d := NewDecompressor(buf, 22, crc(), false)
	n, err := d.WriteTo(unpackedBuf)
	if err != nil {
		t.Error(err, "unpacking")
	} else if int(n) != len(a) {
		t.Error("expected to unpack", len(a), "bytes, unpacked", n)
	} else if !bytes.Equal(a, unpackedBuf.Bytes()) {
		t.Error("decompressed repeats did not match original")
	}
}

// Tests copies whose sources or dests wrap around the ring, and also longish Write()s
func TestRingWrap(t *testing.T) {
	// a is random, but with 1000 bytes we use to make one match whose output wraps
	// around the ring and another whose input does
	ringSize := 1 << CompHistBits
	a := make([]byte, ringSize+1000)
	rndSource, err := rc4.NewCipher([]byte("hello"))
	if err != nil {
		t.Error("couldn't set up garbage source")
	}
	rndSource.XORKeyStream(a, a)

	// the two repeats
	copy(a[ringSize-200:ringSize+200], a[100000:])
	copy(a[ringSize+600:], a[ringSize-200:ringSize+200])

	buf := new(bytes.Buffer)
	c := NewCompressor(buf, crc())
	_, err = c.Write(a)
	if err != nil {
		t.Error(err)
	} else if err = c.Close(); err != nil {
		t.Error(err)
	}

	if buf.Len() > len(a)-500 {
		t.Error("compression only saved", len(a)-buf.Len(), "bytes")
	}

	b := new(bytes.Buffer)
	d := NewDecompressor(buf, 22, crc(), false)
	n, err := d.WriteTo(b)
	if err != nil {
		t.Error(err, "unpacking")
	} else if int(n) != len(a) {
		t.Error("unpacked", n, "bytes, wanted", len(a))
	} else if !bytes.Equal(a, b.Bytes()) {
		t.Error("decompressed does not match original")
	}
}

// these would be nice
func TestDecompressBad(t *testing.T) {
	// copy from too far back
	// copy from the future
	// really long copy
	// really long literal
	// truncated instruction
	// truncated literal
	// truncated copy len
	// ends after instruction without \0
	// ends after \0, truncated checksum
	// ends after \0, missing checksum
	// ends after \0, bad checksum
	// ends after \0, but without empty block when concat=true
}
