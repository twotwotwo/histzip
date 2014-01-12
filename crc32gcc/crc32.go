// A mimic of hash/crc32 using SSE intrinsics for Castagnoli under gccgo.
package crc32

import (
	"hash"
	"hash/crc32"
)

const Castagnoli, IEEE, Koopman = crc32.Castagnoli, crc32.IEEE, crc32.Koopman

// copying these from hash/crc32 (means Update/Checksum don't use SSE)
var MakeTable, Update, Checksum, NewIEEE, IEEETable = crc32.MakeTable, crc32.Update, crc32.Checksum, crc32.NewIEEE, crc32.IEEETable

type crc uint32

//extern crc32
func intrinsic_crc32(crc crc, data *uint8, count int) crc

//extern can_crc32
func can_intrinsic_crc32() uint32

func (c *crc) Write(p []byte) (n int, err error) {
	if len(p) > 0 {
		*c = ^intrinsic_crc32(^*c, &p[0], len(p))
	}
	return len(p), nil
}

func (c crc) Sum32() uint32 { return uint32(c) }

func (c crc) Size() int { return 4 }

func (c crc) BlockSize() int { return 1 }

func (c *crc) Reset() { *c = 0 }

func (c crc) Sum(p []byte) []byte {
	return append(p, byte(c>>24), byte(c>>16), byte(c>>8), byte(c))
}

func New(t *crc32.Table) hash.Hash32 {
	ct := crc32.MakeTable(crc32.Castagnoli)
	if t == ct && can_intrinsic_crc32() != 0 {
		return new(crc)
	}
	return crc32.New(t)
}
