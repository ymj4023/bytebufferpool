# Buffer

The Buffer context defines the append-oriented value offered by bytebufferpool.

## Language

**Buffer**:
A non-copyable append-oriented value that exclusively owns a Pool Lease until it grows or is released.
_Avoid_: ByteBuffer, `bytes.Buffer`, writer cache
