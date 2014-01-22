The histzip framing format consists of:

* The format signature, bytes AC 9A DC F0.

* Bytes with the VerMajor and VerMinor, currently 00 (major) 02 (minor). 
  Decompressors have to reject files with higher major versions than
  they were written for, and accept files with higher minor versions.

* A byte representing `histBits`, the base-2 logarithm of the size of the
  history buffer, which for histzip currently defaults to 22 (0x16), meaning 
  `1<<22` bytes or 4 MB of RAM is needed for decompression. Decompressors 
  should handle `histBits` between 20 and 26 (buffer sizes 1 to 64 MB) at least,
  and can refuse to allocate tons of RAM if histBits is higher.

* A byte representing the number of bytes of extra data that follow. Decompressors 
  should skip any extra bytes; they could be used to add metadata in a 
  backwards-compatible way in the future.
	
* One or more [lrcompress format] blocks, terminated by an empty block.

[lrcompress format]: lrcompress/format.md

Future versions may use the "extra data" in the header or append content after the 
lrcompress data to extend the format without breaking backwards compatibility.	
