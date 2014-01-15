lrcompress is the long-range compression format used in histzip, also usable as a standalone library. This is the format from the perspective of a decompressor:

* The decompressor tracks the last `1<<HistBits` bytes of history (for
  example, 4 MB if `HistBits` is 22).  The framing format/application is
  responsible for providing the right value for `HistBits`.

* The decompressor state also keeps track of a byte offset we'll call `CopyOffset`, 
  initially zero. 

* lrcompress uses [Protocol Buffers-style variable-length integers][varints], or 
  varints. Except for the checksum, all the numbers below are stored as signed 
  varints.

[varints]: https://developers.google.com/protocol-buffers/docs/encoding

* Each block is a series of instructions, either literal, copy, or
  end-of-block.  To read an instruction, the decompressor first reads an
  integer, and looks at its sign to figure out how to proceed: literals are
  negative, copies are positive, and end-of-block is zero.

* A literal instruction starts with a negative integer indicating the number
  of literal bytes following; for example, -5 means the next five bytes 
  from the input should appear in the output.

* Copy instructions begin with a positive integer (`Length`) indicating the
  number of bytes to copy.  That's followed by another integer (`Advance`)
  that might be positive or negative. `Advance` is subtracted from `CopyOffset`,
  then `Length` bytes are output starting from `CopyOffset` bytes ago 
  in the history. In pseudocode:
  
  ```
read signed ints Length and Advance
CopyOffset -= Advance
output Length bytes from CopyOffset bytes ago in the history
  ```

* A copy instruction's source may overlap its destination. In that case,
  decompressors must produce repeats.  For example, a copy with length 5 
  starting 2 bytes before the current output position should output "ababa" if the
  last two bytes were "ab".  This is the output you'd get from a naive loop copying 
  one byte at a time (but not what you'd get from, for instance, memmove).

* 'end-of-block' instructions are a zero, followed by a checksum, which is an xxHash 
  sum with seed 0 written in big-endian order. At end of block, the checksum state is 
  reset but not the (de)compressor state. An empty block marks the end of the stream.
  Compressors must be sure not to write zero-length copies or literals, or they'll be 
  misread as end-of-block markers, and not to write empty blocks before end of stream. 

* Some little things:

	* Extra content after the empty block marking the end-of-stream is OK, and might
	  actually be used by future versions of histzip. The decompressor also shouldn't 
	  count on input containing consistently sized blocks, or blocks small enough 
	  that the unpacked data fits in memory. 

	* Corrupt input might have copy instructions with a nonsense starting 
	  offset: before the beginning of the file, or more than a history buffer 
	  length ago, or "in the future" (i.e., after the last output byte). The 
	  decompressor should cleanly throw an error on reading any of those.
	
	* The decompressor may also error on reading a copy or literal length greater
	  than the size of the history buffer. Allowing huge lengths doesn't achieve 
	  noticeably better compression anyway, and erroring out prevents writing 
	  GBs of uncompressed output because a few corrupt bytes were read as a long 
	  copy. The decompressor can also optimize for the shorter max lengths that 
	  histzip currently emits: 64kb for literals, 256kb for copies. 
	
	* End of file in the middle of a block is an error.
	
	* It should go without saying, but files and blocks over 4GB need to be supported,
	  including by builds for 32-bit architectures.

* If you're reading the histzip source, note that instead of tracking `CopyOffset`, 
  histzip tracks a value it calls `cursor`, which is the current output position minus 
  `CopyOffset`. It also uses the name  `cursorMove` for what this description calls 
  `Advance`. The results are  the same. 

* The framing format/application is responsible for everything not covered here, such 
  as any magic numbers, versioning, and metadata.
