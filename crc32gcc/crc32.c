#include <cpuid.h>

unsigned long long crc32_uint64s(unsigned long long crc, unsigned long long* data, int count) {
  while (count-- > 0) crc = __builtin_ia32_crc32di(crc, *data++);
  return crc;
}

unsigned int crc32_bytes(unsigned int crc, unsigned char* data, int count) {
  while (count-- > 0) crc = __builtin_ia32_crc32qi(crc, *data++);
  return crc;
}

unsigned int crc32(unsigned int crc, unsigned char* data, int count) {
  /* divide into aligned/unalifned parts */
  int leadingBytes = 8-((long)data)&7;
  int trailingBytes = ((long)data+count)&7;
  if ( count < 8 ) {
    leadingBytes = 0;
    trailingBytes = count;
  }
  int alignedWords = (count - leadingBytes)>>3;

  /* do leading part */
  if ( leadingBytes > 0 ) {
    crc = crc32_bytes(crc, data, leadingBytes);
    data += leadingBytes;
  }

  /* do aligned */
  if ( alignedWords > 0 ) {
    unsigned long long* lData = (unsigned long long*)data;
    crc = crc32_uint64s(crc, lData, alignedWords);
    data += alignedWords*8;
  }

  /* do trailing */
  if ( trailingBytes > 0 )
    crc = crc32_bytes(crc, data, trailingBytes);
    
  return crc;
}

int can_crc32() {
  unsigned int cx, dummy;
  __get_cpuid(1, &dummy, &dummy, &cx, &dummy);
  return (cx&1<<20) != 0;
}