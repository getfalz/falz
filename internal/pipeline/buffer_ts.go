package pipeline

// @ts namespace global
// @doc
// Extends the global scope with the Buffer constructor and BufferEncoding type
// as defined by the Node.js buffer module.

// @ts type global.WithImplicitCoercion<T>
//
// @doc
// A value that can be coerced into T, either directly, via `valueOf()`, or via
// a string `Symbol.toPrimitive` hint. Used by several `Buffer.from` overloads.
//
// @pred T | { valueOf(): T } | (T extends string ? { [Symbol.toPrimitive](hint: "string"): T } : never)

// ---------------------------------------------------------------------------
// type: BufferEncoding
// ---------------------------------------------------------------------------

// @ts type global.BufferEncoding
// @doc
// The set of character encodings supported by Buffer operations in this runtime.
// Note: ascii, utf16le, ucs2, latin1, and binary are not supported.
// @pred "utf8" | "utf-8" | "base64" | "base64url" | "hex"

// ---------------------------------------------------------------------------
// interface: BufferConstructor  (static side of Buffer)
// ---------------------------------------------------------------------------

// @ts interface global.BufferConstructor
// @doc
// The constructor interface for Buffer. Exposes static factory methods and
// properties. The global var Buffer is typed as BufferConstructor.
//
// Deprecated constructors (new Buffer(...)) are included for compatibility
// but should not be used; prefer the static factory methods.
//
// @field new(str: string, encoding?: BufferEncoding): Buffer - Allocates a new buffer from a string. Deprecated since v10.0.0 — use Buffer.from(string, encoding) instead.
// @field new(array: ArrayLike<number>): Buffer - Allocates a new buffer from an array of octets. Deprecated since v10.0.0 — use Buffer.from(array) instead.
// @field new(arrayBuffer: ArrayBuffer): Buffer - Produces a Buffer backed by the same memory as the given ArrayBuffer. Deprecated since v10.0.0 — use Buffer.from(arrayBuffer) instead.
// @field from(array: WithImplicitCoercion<ArrayLike<number>>): Buffer - Creates a Buffer from an array of bytes in the range 0–255. Array entries outside that range are truncated.
// @field from(arrayBuffer: ArrayBuffer, byteOffset?: number, length?: number): Buffer - Creates a view of the ArrayBuffer without copying memory. Optional byteOffset and length bound the shared range.
// @field from(string: WithImplicitCoercion<string>, encoding?: BufferEncoding): Buffer - Creates a Buffer containing a string. Default encoding is utf8.
// @field isBuffer(obj: any): obj is Buffer - Returns true if obj is a Buffer.
// @field isEncoding(encoding: string): encoding is BufferEncoding - Returns true if encoding is the name of a supported character encoding.
// @field byteLength(string: string | Buffer | ArrayBuffer, encoding?: BufferEncoding): number - Returns the byte length of a string when encoded using the given encoding.
// @field compare(buf1: Uint8Array, buf2: Uint8Array): -1 | 0 | 1 - Compares buf1 to buf2 for sorting purposes. Equivalent to buf1.compare(buf2).
// @field concat(list: readonly Uint8Array[], totalLength?: number): Buffer - Returns a new Buffer which is the concatenation of all Buffer instances in list.
// @field alloc(size: number, fill?: string | Uint8Array | number, encoding?: BufferEncoding): Buffer - Allocates a new zero-filled Buffer of size bytes. If fill is given, the buffer is initialized with buf.fill(fill).
// @field poolSize: number - The size in bytes of pre-allocated internal Buffer instances used for pooling. May be modified.

// ---------------------------------------------------------------------------
// interface: Buffer  (instance side)
// ---------------------------------------------------------------------------

// @ts interface global.Buffer
// @doc
// Raw binary data stored outside the V8 heap. A Buffer is similar to a
// Uint8Array but provides additional methods for reading and writing various
// integer and floating-point formats, as well as string encoding/decoding.
// A Buffer cannot be resized after allocation.
//
// Instance methods for reading/writing integers use the naming convention:
//   read/write + (Big)? + (U)? + Int/Float/Double + (8|16|32|64)? + (LE|BE)?
// Lowercase aliases (e.g. readUint8, writeBigUint64BE) exist for all methods
// that previously used uppercase 'I' in 'UInt'.

// --- string / encoding ---

// @ts method global.Buffer.write
// @doc
// Writes string to buf at offset according to encoding.
// Only part of string is written if buf does not have enough space.
// Partially encoded characters are never written.
// @sig write(string: string, encoding?: BufferEncoding): number
// @returns number - Number of bytes written.

// @ts method global.Buffer.toString
// @doc
// Decodes buf to a string using the specified character encoding.
// start and end may be passed to decode only a subset of buf.
// Invalid UTF-8 bytes are replaced with U+FFFD when encoding is utf8.
// @sig toString(encoding?: BufferEncoding, start?: number, end?: number): string
// @returns string - The decoded string.

// @ts method global.Buffer.equals
// @doc
// Returns true if both buf and otherBuffer have exactly the same bytes.
// Equivalent to buf.compare(otherBuffer) === 0.
// @param otherBuffer - Uint8Array - The Buffer or Uint8Array to compare against.
// @returns boolean

// --- 64-bit big-integer reads ---

// @ts method global.Buffer.readBigUInt64BE
// @doc Reads an unsigned, big-endian 64-bit integer from buf at the specified offset. Also available as readBigUint64BE.
// @param offset? - number - Bytes to skip before reading. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
// @returns bigint

// @ts method global.Buffer.readBigUint64BE
// @doc Alias for readBigUInt64BE.
// @param offset? - number - Bytes to skip before reading. Default 0.
// @returns bigint

// @ts method global.Buffer.readBigUInt64LE
// @doc Reads an unsigned, little-endian 64-bit integer from buf at the specified offset. Also available as readBigUint64LE.
// @param offset? - number - Bytes to skip before reading. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
// @returns bigint

// @ts method global.Buffer.readBigUint64LE
// @doc Alias for readBigUInt64LE.
// @param offset? - number - Bytes to skip before reading. Default 0.
// @returns bigint

// @ts method global.Buffer.readBigInt64BE
// @doc Reads a signed, big-endian 64-bit integer from buf at the specified offset. Interpreted as two's complement.
// @param offset? - number - Bytes to skip before reading. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
// @returns bigint

// @ts method global.Buffer.readBigInt64LE
// @doc Reads a signed, little-endian 64-bit integer from buf at the specified offset. Interpreted as two's complement.
// @param offset? - number - Bytes to skip before reading. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
// @returns bigint

// --- variable-width integer reads ---

// @ts method global.Buffer.readUIntLE
// @doc Reads byteLength bytes from buf at offset as an unsigned little-endian integer. Supports up to 48-bit accuracy. Also available as readUintLE.
// @param offset - number - Bytes to skip. Must satisfy 0 <= offset <= buf.length - byteLength.
// @param byteLength - number - Bytes to read. Must satisfy 0 < byteLength <= 6.
// @returns number

// @ts method global.Buffer.readUintLE
// @doc Alias for readUIntLE.
// @param offset - number - Bytes to skip.
// @param byteLength - number - Bytes to read.
// @returns number

// @ts method global.Buffer.readUIntBE
// @doc Reads byteLength bytes from buf at offset as an unsigned big-endian integer. Supports up to 48-bit accuracy. Also available as readUintBE.
// @param offset - number - Bytes to skip. Must satisfy 0 <= offset <= buf.length - byteLength.
// @param byteLength - number - Bytes to read. Must satisfy 0 < byteLength <= 6.
// @returns number

// @ts method global.Buffer.readUintBE
// @doc Alias for readUIntBE.
// @param offset - number - Bytes to skip.
// @param byteLength - number - Bytes to read.
// @returns number

// @ts method global.Buffer.readIntLE
// @doc Reads byteLength bytes from buf at offset as a little-endian two's complement signed integer. Supports up to 48-bit accuracy.
// @param offset - number - Bytes to skip. Must satisfy 0 <= offset <= buf.length - byteLength.
// @param byteLength - number - Bytes to read. Must satisfy 0 < byteLength <= 6.
// @returns number

// @ts method global.Buffer.readIntBE
// @doc Reads byteLength bytes from buf at offset as a big-endian two's complement signed integer. Supports up to 48-bit accuracy.
// @param offset - number - Bytes to skip. Must satisfy 0 <= offset <= buf.length - byteLength.
// @param byteLength - number - Bytes to read. Must satisfy 0 < byteLength <= 6.
// @returns number

// --- fixed-width unsigned integer reads ---

// @ts method global.Buffer.readUInt8
// @doc Reads an unsigned 8-bit integer from buf at offset. Also available as readUint8.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 1.
// @returns number

// @ts method global.Buffer.readUint8
// @doc Alias for readUInt8.
// @param offset? - number - Bytes to skip. Default 0.
// @returns number

// @ts method global.Buffer.readUInt16LE
// @doc Reads an unsigned, little-endian 16-bit integer from buf at offset. Also available as readUint16LE.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 2.
// @returns number

// @ts method global.Buffer.readUint16LE
// @doc Alias for readUInt16LE.
// @param offset? - number - Bytes to skip. Default 0.
// @returns number

// @ts method global.Buffer.readUInt16BE
// @doc Reads an unsigned, big-endian 16-bit integer from buf at offset. Also available as readUint16BE.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 2.
// @returns number

// @ts method global.Buffer.readUint16BE
// @doc Alias for readUInt16BE.
// @param offset? - number - Bytes to skip. Default 0.
// @returns number

// @ts method global.Buffer.readUInt32LE
// @doc Reads an unsigned, little-endian 32-bit integer from buf at offset. Also available as readUint32LE.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
// @returns number

// @ts method global.Buffer.readUint32LE
// @doc Alias for readUInt32LE.
// @param offset? - number - Bytes to skip. Default 0.
// @returns number

// @ts method global.Buffer.readUInt32BE
// @doc Reads an unsigned, big-endian 32-bit integer from buf at offset. Also available as readUint32BE.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
// @returns number

// @ts method global.Buffer.readUint32BE
// @doc Alias for readUInt32BE.
// @param offset? - number - Bytes to skip. Default 0.
// @returns number

// --- fixed-width signed integer reads ---

// @ts method global.Buffer.readInt8
// @doc Reads a signed 8-bit integer from buf at offset. Interpreted as two's complement.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 1.
// @returns number

// @ts method global.Buffer.readInt16LE
// @doc Reads a signed, little-endian 16-bit integer from buf at offset. Interpreted as two's complement.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 2.
// @returns number

// @ts method global.Buffer.readInt16BE
// @doc Reads a signed, big-endian 16-bit integer from buf at offset. Interpreted as two's complement.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 2.
// @returns number

// @ts method global.Buffer.readInt32LE
// @doc Reads a signed, little-endian 32-bit integer from buf at offset. Interpreted as two's complement.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
// @returns number

// @ts method global.Buffer.readInt32BE
// @doc Reads a signed, big-endian 32-bit integer from buf at offset. Interpreted as two's complement.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
// @returns number

// --- float / double reads ---

// @ts method global.Buffer.readFloatLE
// @doc Reads a 32-bit, little-endian float from buf at offset.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
// @returns number

// @ts method global.Buffer.readFloatBE
// @doc Reads a 32-bit, big-endian float from buf at offset.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
// @returns number

// @ts method global.Buffer.readDoubleLE
// @doc Reads a 64-bit, little-endian double from buf at offset.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
// @returns number

// @ts method global.Buffer.readDoubleBE
// @doc Reads a 64-bit, big-endian double from buf at offset.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
// @returns number

// --- 64-bit big-integer writes ---

// @ts method global.Buffer.writeBigInt64BE
// @doc Writes value to buf at offset as a big-endian two's complement signed 64-bit integer.
// @param value - bigint - Value to write.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeBigInt64LE
// @doc Writes value to buf at offset as a little-endian two's complement signed 64-bit integer.
// @param value - bigint - Value to write.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeBigUInt64BE
// @doc Writes value to buf at offset as a big-endian unsigned 64-bit integer. Also available as writeBigUint64BE.
// @param value - bigint - Value to write.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeBigUint64BE
// @doc Alias for writeBigUInt64BE.
// @param value - bigint - Value to write.
// @param offset? - number - Bytes to skip. Default 0.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeBigUInt64LE
// @doc Writes value to buf at offset as a little-endian unsigned 64-bit integer. Also available as writeBigUint64LE.
// @param value - bigint - Value to write.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeBigUint64LE
// @doc Alias for writeBigUInt64LE.
// @param value - bigint - Value to write.
// @param offset? - number - Bytes to skip. Default 0.
// @returns number - offset plus the number of bytes written.

// --- variable-width integer writes ---

// @ts method global.Buffer.writeUIntLE
// @doc Writes byteLength bytes of value to buf at offset as little-endian. Supports up to 48-bit accuracy. Also available as writeUintLE.
// @param value - number - Value to write.
// @param offset - number - Bytes to skip. Must satisfy 0 <= offset <= buf.length - byteLength.
// @param byteLength - number - Bytes to write. Must satisfy 0 < byteLength <= 6.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeUintLE
// @doc Alias for writeUIntLE.
// @param value - number - Value to write.
// @param offset - number - Bytes to skip.
// @param byteLength - number - Bytes to write.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeUIntBE
// @doc Writes byteLength bytes of value to buf at offset as big-endian. Supports up to 48-bit accuracy. Also available as writeUintBE.
// @param value - number - Value to write.
// @param offset - number - Bytes to skip. Must satisfy 0 <= offset <= buf.length - byteLength.
// @param byteLength - number - Bytes to write. Must satisfy 0 < byteLength <= 6.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeUintBE
// @doc Alias for writeUIntBE.
// @param value - number - Value to write.
// @param offset - number - Bytes to skip.
// @param byteLength - number - Bytes to write.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeIntLE
// @doc Writes byteLength bytes of value to buf at offset as a little-endian signed integer. Supports up to 48-bit accuracy.
// @param value - number - Value to write.
// @param offset - number - Bytes to skip. Must satisfy 0 <= offset <= buf.length - byteLength.
// @param byteLength - number - Bytes to write. Must satisfy 0 < byteLength <= 6.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeIntBE
// @doc Writes byteLength bytes of value to buf at offset as a big-endian signed integer. Supports up to 48-bit accuracy.
// @param value - number - Value to write.
// @param offset - number - Bytes to skip. Must satisfy 0 <= offset <= buf.length - byteLength.
// @param byteLength - number - Bytes to write. Must satisfy 0 < byteLength <= 6.
// @returns number - offset plus the number of bytes written.

// --- fixed-width unsigned integer writes ---

// @ts method global.Buffer.writeUInt8
// @doc Writes an unsigned 8-bit integer value to buf at offset. Also available as writeUint8.
// @param value - number - Value to write. Must be a valid unsigned 8-bit integer.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 1.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeUint8
// @doc Alias for writeUInt8.
// @param value - number - Value to write.
// @param offset? - number - Bytes to skip. Default 0.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeUInt16LE
// @doc Writes an unsigned 16-bit integer value to buf at offset as little-endian. Also available as writeUint16LE.
// @param value - number - Value to write. Must be a valid unsigned 16-bit integer.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 2.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeUint16LE
// @doc Alias for writeUInt16LE.
// @param value - number - Value to write.
// @param offset? - number - Bytes to skip. Default 0.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeUInt16BE
// @doc Writes an unsigned 16-bit integer value to buf at offset as big-endian. Also available as writeUint16BE.
// @param value - number - Value to write. Must be a valid unsigned 16-bit integer.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 2.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeUint16BE
// @doc Alias for writeUInt16BE.
// @param value - number - Value to write.
// @param offset? - number - Bytes to skip. Default 0.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeUInt32LE
// @doc Writes an unsigned 32-bit integer value to buf at offset as little-endian. Also available as writeUint32LE.
// @param value - number - Value to write. Must be a valid unsigned 32-bit integer.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeUint32LE
// @doc Alias for writeUInt32LE.
// @param value - number - Value to write.
// @param offset? - number - Bytes to skip. Default 0.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeUInt32BE
// @doc Writes an unsigned 32-bit integer value to buf at offset as big-endian. Also available as writeUint32BE.
// @param value - number - Value to write. Must be a valid unsigned 32-bit integer.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeUint32BE
// @doc Alias for writeUInt32BE.
// @param value - number - Value to write.
// @param offset? - number - Bytes to skip. Default 0.
// @returns number - offset plus the number of bytes written.

// --- fixed-width signed integer writes ---

// @ts method global.Buffer.writeInt8
// @doc Writes a signed 8-bit integer value to buf at offset. Interpreted as two's complement.
// @param value - number - Value to write. Must be a valid signed 8-bit integer.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 1.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeInt16LE
// @doc Writes a signed 16-bit integer value to buf at offset as little-endian. Interpreted as two's complement.
// @param value - number - Value to write. Must be a valid signed 16-bit integer.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 2.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeInt16BE
// @doc Writes a signed 16-bit integer value to buf at offset as big-endian. Interpreted as two's complement.
// @param value - number - Value to write. Must be a valid signed 16-bit integer.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 2.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeInt32LE
// @doc Writes a signed 32-bit integer value to buf at offset as little-endian. Interpreted as two's complement.
// @param value - number - Value to write. Must be a valid signed 32-bit integer.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeInt32BE
// @doc Writes a signed 32-bit integer value to buf at offset as big-endian. Interpreted as two's complement.
// @param value - number - Value to write. Must be a valid signed 32-bit integer.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
// @returns number - offset plus the number of bytes written.

// --- float / double writes ---

// @ts method global.Buffer.writeFloatLE
// @doc Writes a 32-bit float value to buf at offset as little-endian.
// @param value - number - Value to write.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeFloatBE
// @doc Writes a 32-bit float value to buf at offset as big-endian.
// @param value - number - Value to write.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeDoubleLE
// @doc Writes a 64-bit double value to buf at offset as little-endian.
// @param value - number - Value to write.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
// @returns number - offset plus the number of bytes written.

// @ts method global.Buffer.writeDoubleBE
// @doc Writes a 64-bit double value to buf at offset as big-endian.
// @param value - number - Value to write.
// @param offset? - number - Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
// @returns number - offset plus the number of bytes written.

// ---------------------------------------------------------------------------
// global var: Buffer
// ---------------------------------------------------------------------------

// @ts var global.Buffer
// @type BufferConstructor
// @doc
// The global Buffer constructor. Use Buffer.from(), Buffer.alloc(), or
// Buffer.concat() to create Buffer instances. Direct use of new Buffer()
// is deprecated since Node.js v10.0.0.
