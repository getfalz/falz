/**
 * 
 * Extends the global scope with the Buffer constructor and BufferEncoding type
 * as defined by the Node.js buffer module.
 *
 */
export {};
declare global {
  /**
   * 
   * The constructor interface for Buffer. Exposes static factory methods and
   * properties. The global var Buffer is typed as BufferConstructor.
   * 
   * Deprecated constructors (new Buffer(...)) are included for compatibility
   * but should not be used; prefer the static factory methods.
   * 
   *
   */
  interface BufferConstructor {
    /**
     * Allocates a new buffer from a string. Deprecated since v10.0.0 — use Buffer.from(string, encoding) instead.
     */
    new(str: string, encoding?: BufferEncoding): Buffer
    /**
     * Allocates a new buffer from an array of octets. Deprecated since v10.0.0 — use Buffer.from(array) instead.
     */
    new(array: ArrayLike<number>): Buffer
    /**
     * Produces a Buffer backed by the same memory as the given ArrayBuffer. Deprecated since v10.0.0 — use Buffer.from(arrayBuffer) instead.
     */
    new(arrayBuffer: ArrayBuffer): Buffer
    /**
     * Creates a Buffer from an array of bytes in the range 0–255. Array entries outside that range are truncated.
     */
    from(array: WithImplicitCoercion<ArrayLike<number>>): Buffer
    /**
     * Creates a view of the ArrayBuffer without copying memory. Optional byteOffset and length bound the shared range.
     */
    from(arrayBuffer: ArrayBuffer, byteOffset?: number, length?: number): Buffer
    /**
     * Creates a Buffer containing a string. Default encoding is utf8.
     */
    from(string: WithImplicitCoercion<string>, encoding?: BufferEncoding): Buffer
    /**
     * Returns true if obj is a Buffer.
     */
    isBuffer(obj: any): obj is Buffer
    /**
     * Returns true if encoding is the name of a supported character encoding.
     */
    isEncoding(encoding: string): encoding is BufferEncoding
    /**
     * Returns the byte length of a string when encoded using the given encoding.
     */
    byteLength(string: string | Buffer | ArrayBuffer, encoding?: BufferEncoding): number
    /**
     * Compares buf1 to buf2 for sorting purposes. Equivalent to buf1.compare(buf2).
     */
    compare(buf1: Uint8Array, buf2: Uint8Array): -1 | 0 | 1
    /**
     * Returns a new Buffer which is the concatenation of all Buffer instances in list.
     */
    concat(list: readonly Uint8Array[], totalLength?: number): Buffer
    /**
     * Allocates a new zero-filled Buffer of size bytes. If fill is given, the buffer is initialized with buf.fill(fill).
     */
    alloc(size: number, fill?: string | Uint8Array | number, encoding?: BufferEncoding): Buffer
    /**
     * The size in bytes of pre-allocated internal Buffer instances used for pooling. May be modified.
     */
    poolSize: number
  }
  /**
   * 
   * Raw binary data stored outside the V8 heap. A Buffer is similar to a
   * Uint8Array but provides additional methods for reading and writing various
   * integer and floating-point formats, as well as string encoding/decoding.
   * A Buffer cannot be resized after allocation.
   * 
   * Instance methods for reading/writing integers use the naming convention:
   * read/write + (Big)? + (U)? + Int/Float/Double + (8|16|32|64)? + (LE|BE)?
   * Lowercase aliases (e.g. readUint8, writeBigUint64BE) exist for all methods
   * that previously used uppercase 'I' in 'UInt'.
   *
   */
  interface Buffer {
    /**
     * 
     * Writes string to buf at offset according to encoding.
     * Only part of string is written if buf does not have enough space.
     * Partially encoded characters are never written.
     * @returns Number of bytes written.
     *
     */
    write(string: string, encoding?: BufferEncoding): number
    /**
     * 
     * Decodes buf to a string using the specified character encoding.
     * start and end may be passed to decode only a subset of buf.
     * Invalid UTF-8 bytes are replaced with U+FFFD when encoding is utf8.
     * @returns The decoded string.
     *
     */
    toString(encoding?: BufferEncoding, start?: number, end?: number): string
    /**
     * 
     * Returns true if both buf and otherBuffer have exactly the same bytes.
     * Equivalent to buf.compare(otherBuffer) === 0.
     * @param otherBuffer The Buffer or Uint8Array to compare against.
     *
     */
    equals(otherBuffer: Uint8Array): boolean
    /**
     * Reads an unsigned, big-endian 64-bit integer from buf at the specified offset. Also available as readBigUint64BE.
     * @param offset? Bytes to skip before reading. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
     *
     */
    readBigUInt64BE(offset?: number): bigint
    /**
     * Alias for readBigUInt64BE.
     * @param offset? Bytes to skip before reading. Default 0.
     *
     */
    readBigUint64BE(offset?: number): bigint
    /**
     * Reads an unsigned, little-endian 64-bit integer from buf at the specified offset. Also available as readBigUint64LE.
     * @param offset? Bytes to skip before reading. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
     *
     */
    readBigUInt64LE(offset?: number): bigint
    /**
     * Alias for readBigUInt64LE.
     * @param offset? Bytes to skip before reading. Default 0.
     *
     */
    readBigUint64LE(offset?: number): bigint
    /**
     * Reads a signed, big-endian 64-bit integer from buf at the specified offset. Interpreted as two's complement.
     * @param offset? Bytes to skip before reading. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
     *
     */
    readBigInt64BE(offset?: number): bigint
    /**
     * Reads a signed, little-endian 64-bit integer from buf at the specified offset. Interpreted as two's complement.
     * @param offset? Bytes to skip before reading. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
     *
     */
    readBigInt64LE(offset?: number): bigint
    /**
     * Reads byteLength bytes from buf at offset as an unsigned little-endian integer. Supports up to 48-bit accuracy. Also available as readUintLE.
     * @param offset Bytes to skip. Must satisfy 0 <= offset <= buf.length - byteLength.
     * @param byteLength Bytes to read. Must satisfy 0 < byteLength <= 6.
     *
     */
    readUIntLE(offset: number, byteLength: number): number
    /**
     * Alias for readUIntLE.
     * @param offset Bytes to skip.
     * @param byteLength Bytes to read.
     *
     */
    readUintLE(offset: number, byteLength: number): number
    /**
     * Reads byteLength bytes from buf at offset as an unsigned big-endian integer. Supports up to 48-bit accuracy. Also available as readUintBE.
     * @param offset Bytes to skip. Must satisfy 0 <= offset <= buf.length - byteLength.
     * @param byteLength Bytes to read. Must satisfy 0 < byteLength <= 6.
     *
     */
    readUIntBE(offset: number, byteLength: number): number
    /**
     * Alias for readUIntBE.
     * @param offset Bytes to skip.
     * @param byteLength Bytes to read.
     *
     */
    readUintBE(offset: number, byteLength: number): number
    /**
     * Reads byteLength bytes from buf at offset as a little-endian two's complement signed integer. Supports up to 48-bit accuracy.
     * @param offset Bytes to skip. Must satisfy 0 <= offset <= buf.length - byteLength.
     * @param byteLength Bytes to read. Must satisfy 0 < byteLength <= 6.
     *
     */
    readIntLE(offset: number, byteLength: number): number
    /**
     * Reads byteLength bytes from buf at offset as a big-endian two's complement signed integer. Supports up to 48-bit accuracy.
     * @param offset Bytes to skip. Must satisfy 0 <= offset <= buf.length - byteLength.
     * @param byteLength Bytes to read. Must satisfy 0 < byteLength <= 6.
     *
     */
    readIntBE(offset: number, byteLength: number): number
    /**
     * Reads an unsigned 8-bit integer from buf at offset. Also available as readUint8.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 1.
     *
     */
    readUInt8(offset?: number): number
    /**
     * Alias for readUInt8.
     * @param offset? Bytes to skip. Default 0.
     *
     */
    readUint8(offset?: number): number
    /**
     * Reads an unsigned, little-endian 16-bit integer from buf at offset. Also available as readUint16LE.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 2.
     *
     */
    readUInt16LE(offset?: number): number
    /**
     * Alias for readUInt16LE.
     * @param offset? Bytes to skip. Default 0.
     *
     */
    readUint16LE(offset?: number): number
    /**
     * Reads an unsigned, big-endian 16-bit integer from buf at offset. Also available as readUint16BE.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 2.
     *
     */
    readUInt16BE(offset?: number): number
    /**
     * Alias for readUInt16BE.
     * @param offset? Bytes to skip. Default 0.
     *
     */
    readUint16BE(offset?: number): number
    /**
     * Reads an unsigned, little-endian 32-bit integer from buf at offset. Also available as readUint32LE.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
     *
     */
    readUInt32LE(offset?: number): number
    /**
     * Alias for readUInt32LE.
     * @param offset? Bytes to skip. Default 0.
     *
     */
    readUint32LE(offset?: number): number
    /**
     * Reads an unsigned, big-endian 32-bit integer from buf at offset. Also available as readUint32BE.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
     *
     */
    readUInt32BE(offset?: number): number
    /**
     * Alias for readUInt32BE.
     * @param offset? Bytes to skip. Default 0.
     *
     */
    readUint32BE(offset?: number): number
    /**
     * Reads a signed 8-bit integer from buf at offset. Interpreted as two's complement.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 1.
     *
     */
    readInt8(offset?: number): number
    /**
     * Reads a signed, little-endian 16-bit integer from buf at offset. Interpreted as two's complement.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 2.
     *
     */
    readInt16LE(offset?: number): number
    /**
     * Reads a signed, big-endian 16-bit integer from buf at offset. Interpreted as two's complement.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 2.
     *
     */
    readInt16BE(offset?: number): number
    /**
     * Reads a signed, little-endian 32-bit integer from buf at offset. Interpreted as two's complement.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
     *
     */
    readInt32LE(offset?: number): number
    /**
     * Reads a signed, big-endian 32-bit integer from buf at offset. Interpreted as two's complement.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
     *
     */
    readInt32BE(offset?: number): number
    /**
     * Reads a 32-bit, little-endian float from buf at offset.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
     *
     */
    readFloatLE(offset?: number): number
    /**
     * Reads a 32-bit, big-endian float from buf at offset.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
     *
     */
    readFloatBE(offset?: number): number
    /**
     * Reads a 64-bit, little-endian double from buf at offset.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
     *
     */
    readDoubleLE(offset?: number): number
    /**
     * Reads a 64-bit, big-endian double from buf at offset.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
     *
     */
    readDoubleBE(offset?: number): number
    /**
     * Writes value to buf at offset as a big-endian two's complement signed 64-bit integer.
     * @param value Value to write.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
     * @returns offset plus the number of bytes written.
     *
     */
    writeBigInt64BE(value: bigint, offset?: number): number
    /**
     * Writes value to buf at offset as a little-endian two's complement signed 64-bit integer.
     * @param value Value to write.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
     * @returns offset plus the number of bytes written.
     *
     */
    writeBigInt64LE(value: bigint, offset?: number): number
    /**
     * Writes value to buf at offset as a big-endian unsigned 64-bit integer. Also available as writeBigUint64BE.
     * @param value Value to write.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
     * @returns offset plus the number of bytes written.
     *
     */
    writeBigUInt64BE(value: bigint, offset?: number): number
    /**
     * Alias for writeBigUInt64BE.
     * @param value Value to write.
     * @param offset? Bytes to skip. Default 0.
     * @returns offset plus the number of bytes written.
     *
     */
    writeBigUint64BE(value: bigint, offset?: number): number
    /**
     * Writes value to buf at offset as a little-endian unsigned 64-bit integer. Also available as writeBigUint64LE.
     * @param value Value to write.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
     * @returns offset plus the number of bytes written.
     *
     */
    writeBigUInt64LE(value: bigint, offset?: number): number
    /**
     * Alias for writeBigUInt64LE.
     * @param value Value to write.
     * @param offset? Bytes to skip. Default 0.
     * @returns offset plus the number of bytes written.
     *
     */
    writeBigUint64LE(value: bigint, offset?: number): number
    /**
     * Writes byteLength bytes of value to buf at offset as little-endian. Supports up to 48-bit accuracy. Also available as writeUintLE.
     * @param value Value to write.
     * @param offset Bytes to skip. Must satisfy 0 <= offset <= buf.length - byteLength.
     * @param byteLength Bytes to write. Must satisfy 0 < byteLength <= 6.
     * @returns offset plus the number of bytes written.
     *
     */
    writeUIntLE(value: number, offset: number, byteLength: number): number
    /**
     * Alias for writeUIntLE.
     * @param value Value to write.
     * @param offset Bytes to skip.
     * @param byteLength Bytes to write.
     * @returns offset plus the number of bytes written.
     *
     */
    writeUintLE(value: number, offset: number, byteLength: number): number
    /**
     * Writes byteLength bytes of value to buf at offset as big-endian. Supports up to 48-bit accuracy. Also available as writeUintBE.
     * @param value Value to write.
     * @param offset Bytes to skip. Must satisfy 0 <= offset <= buf.length - byteLength.
     * @param byteLength Bytes to write. Must satisfy 0 < byteLength <= 6.
     * @returns offset plus the number of bytes written.
     *
     */
    writeUIntBE(value: number, offset: number, byteLength: number): number
    /**
     * Alias for writeUIntBE.
     * @param value Value to write.
     * @param offset Bytes to skip.
     * @param byteLength Bytes to write.
     * @returns offset plus the number of bytes written.
     *
     */
    writeUintBE(value: number, offset: number, byteLength: number): number
    /**
     * Writes byteLength bytes of value to buf at offset as a little-endian signed integer. Supports up to 48-bit accuracy.
     * @param value Value to write.
     * @param offset Bytes to skip. Must satisfy 0 <= offset <= buf.length - byteLength.
     * @param byteLength Bytes to write. Must satisfy 0 < byteLength <= 6.
     * @returns offset plus the number of bytes written.
     *
     */
    writeIntLE(value: number, offset: number, byteLength: number): number
    /**
     * Writes byteLength bytes of value to buf at offset as a big-endian signed integer. Supports up to 48-bit accuracy.
     * @param value Value to write.
     * @param offset Bytes to skip. Must satisfy 0 <= offset <= buf.length - byteLength.
     * @param byteLength Bytes to write. Must satisfy 0 < byteLength <= 6.
     * @returns offset plus the number of bytes written.
     *
     */
    writeIntBE(value: number, offset: number, byteLength: number): number
    /**
     * Writes an unsigned 8-bit integer value to buf at offset. Also available as writeUint8.
     * @param value Value to write. Must be a valid unsigned 8-bit integer.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 1.
     * @returns offset plus the number of bytes written.
     *
     */
    writeUInt8(value: number, offset?: number): number
    /**
     * Alias for writeUInt8.
     * @param value Value to write.
     * @param offset? Bytes to skip. Default 0.
     * @returns offset plus the number of bytes written.
     *
     */
    writeUint8(value: number, offset?: number): number
    /**
     * Writes an unsigned 16-bit integer value to buf at offset as little-endian. Also available as writeUint16LE.
     * @param value Value to write. Must be a valid unsigned 16-bit integer.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 2.
     * @returns offset plus the number of bytes written.
     *
     */
    writeUInt16LE(value: number, offset?: number): number
    /**
     * Alias for writeUInt16LE.
     * @param value Value to write.
     * @param offset? Bytes to skip. Default 0.
     * @returns offset plus the number of bytes written.
     *
     */
    writeUint16LE(value: number, offset?: number): number
    /**
     * Writes an unsigned 16-bit integer value to buf at offset as big-endian. Also available as writeUint16BE.
     * @param value Value to write. Must be a valid unsigned 16-bit integer.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 2.
     * @returns offset plus the number of bytes written.
     *
     */
    writeUInt16BE(value: number, offset?: number): number
    /**
     * Alias for writeUInt16BE.
     * @param value Value to write.
     * @param offset? Bytes to skip. Default 0.
     * @returns offset plus the number of bytes written.
     *
     */
    writeUint16BE(value: number, offset?: number): number
    /**
     * Writes an unsigned 32-bit integer value to buf at offset as little-endian. Also available as writeUint32LE.
     * @param value Value to write. Must be a valid unsigned 32-bit integer.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
     * @returns offset plus the number of bytes written.
     *
     */
    writeUInt32LE(value: number, offset?: number): number
    /**
     * Alias for writeUInt32LE.
     * @param value Value to write.
     * @param offset? Bytes to skip. Default 0.
     * @returns offset plus the number of bytes written.
     *
     */
    writeUint32LE(value: number, offset?: number): number
    /**
     * Writes an unsigned 32-bit integer value to buf at offset as big-endian. Also available as writeUint32BE.
     * @param value Value to write. Must be a valid unsigned 32-bit integer.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
     * @returns offset plus the number of bytes written.
     *
     */
    writeUInt32BE(value: number, offset?: number): number
    /**
     * Alias for writeUInt32BE.
     * @param value Value to write.
     * @param offset? Bytes to skip. Default 0.
     * @returns offset plus the number of bytes written.
     *
     */
    writeUint32BE(value: number, offset?: number): number
    /**
     * Writes a signed 8-bit integer value to buf at offset. Interpreted as two's complement.
     * @param value Value to write. Must be a valid signed 8-bit integer.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 1.
     * @returns offset plus the number of bytes written.
     *
     */
    writeInt8(value: number, offset?: number): number
    /**
     * Writes a signed 16-bit integer value to buf at offset as little-endian. Interpreted as two's complement.
     * @param value Value to write. Must be a valid signed 16-bit integer.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 2.
     * @returns offset plus the number of bytes written.
     *
     */
    writeInt16LE(value: number, offset?: number): number
    /**
     * Writes a signed 16-bit integer value to buf at offset as big-endian. Interpreted as two's complement.
     * @param value Value to write. Must be a valid signed 16-bit integer.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 2.
     * @returns offset plus the number of bytes written.
     *
     */
    writeInt16BE(value: number, offset?: number): number
    /**
     * Writes a signed 32-bit integer value to buf at offset as little-endian. Interpreted as two's complement.
     * @param value Value to write. Must be a valid signed 32-bit integer.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
     * @returns offset plus the number of bytes written.
     *
     */
    writeInt32LE(value: number, offset?: number): number
    /**
     * Writes a signed 32-bit integer value to buf at offset as big-endian. Interpreted as two's complement.
     * @param value Value to write. Must be a valid signed 32-bit integer.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
     * @returns offset plus the number of bytes written.
     *
     */
    writeInt32BE(value: number, offset?: number): number
    /**
     * Writes a 32-bit float value to buf at offset as little-endian.
     * @param value Value to write.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
     * @returns offset plus the number of bytes written.
     *
     */
    writeFloatLE(value: number, offset?: number): number
    /**
     * Writes a 32-bit float value to buf at offset as big-endian.
     * @param value Value to write.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 4.
     * @returns offset plus the number of bytes written.
     *
     */
    writeFloatBE(value: number, offset?: number): number
    /**
     * Writes a 64-bit double value to buf at offset as little-endian.
     * @param value Value to write.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
     * @returns offset plus the number of bytes written.
     *
     */
    writeDoubleLE(value: number, offset?: number): number
    /**
     * Writes a 64-bit double value to buf at offset as big-endian.
     * @param value Value to write.
     * @param offset? Bytes to skip. Default 0. Must satisfy 0 <= offset <= buf.length - 8.
     * @returns offset plus the number of bytes written.
     *
     */
    writeDoubleBE(value: number, offset?: number): number
  }
  /**
   * An iterator over URLSearchParams entries, keys, or values.
   *
   */
  interface URLSearchParamsIterator<T> extends Iterator<T, BuiltinIteratorReturn, unknown> {
    /**
     * Returns this iterator.
     */
    [Symbol.iterator](): URLSearchParamsIterator<T>
  }
  interface Console {
    /**
     * Logs the provided arguments to the console with info level.
     */
    log(...data: any[]): void
    /**
     * Logs the provided arguments to the console with info level.
     */
    info(...data: any[]): void
    /**
     * Logs the provided arguments to the console with warn level.
     */
    warn(...data: any[]): void
    /**
     * Logs the provided arguments to the console with error level.
     */
    error(...data: any[]): void
    /**
     * Dumps the provided arguments to the console with error level.
     */
    dump(...data: any[]): void
  }
  interface Console {
    /**
     * Logs the provided arguments to the console with info level.
     */
    log(...data: any[]): void
    /**
     * Logs the provided arguments to the console with info level.
     */
    info(...data: any[]): void
    /**
     * Logs the provided arguments to the console with warn level.
     */
    warn(...data: any[]): void
    /**
     * Logs the provided arguments to the console with error level.
     */
    error(...data: any[]): void
    /**
     * Dumps the provided arguments to the console with error level.
     */
    dump(...data: any[]): void
  }
  /**
   * 
   * The global Buffer constructor. Use Buffer.from(), Buffer.alloc(), or
   * Buffer.concat() to create Buffer instances. Direct use of new Buffer()
   * is deprecated since Node.js v10.0.0.
   *
   */
  var Buffer: BufferConstructor
  /**
   * 
   * Returns the Punycode ASCII serialization of the domain.
   * Returns an empty string if the domain is invalid.
   * Performs the inverse operation to domainToUnicode.
   * @param domain The Unicode domain name to convert.
   * @returns The ASCII (Punycode) serialization, or an empty string.
   *
   */
  function domainToASCII (domain: string): string
  /**
   * 
   * Returns the Unicode serialization of the domain.
   * Returns an empty string if the domain is invalid.
   * Performs the inverse operation to domainToASCII.
   * @param domain The Punycode domain name to convert.
   * @returns The Unicode serialization, or an empty string.
   *
   */
  function domainToUnicode (domain: string): string
  /**
   * Console provides a simple logging interface for JavaScript pipelines.
   *
   */
  var console: Console
  /**
   * Console provides a simple logging interface for JavaScript pipelines.
   *
   */
  var console: Console
  /**
   * 
   * Browser-compatible URL class implemented by following the WHATWG URL Standard.
   * All properties are implemented as getters and setters on the class prototype.
   * Using the delete keyword on any property has no effect but still returns true.
   *
   */
  export class URL {
    /**
     * Create a new URL object.
     * @param input The URL string to parse.
     * @param base? Optional base URL used to resolve relative input URLs.
     *
     */
    constructor (input: string | { toString: () => string }, base?: string | URL)
    /**
     * 
     * Gets and sets the fragment portion of the URL (the part after #).
     * Invalid URL characters in the assigned value are percent-encoded.
     *
     */
    hash: string
    /**
     * 
     * Gets and sets the host portion of the URL, including the port.
     * Invalid host values assigned to the property are ignored.
     *
     */
    host: string
    /**
     * 
     * Gets and sets the host name portion of the URL without the port.
     * Invalid host name values assigned to the property are ignored.
     *
     */
    hostname: string
    /**
     * 
     * Gets and sets the serialized URL. Getting this value is equivalent to
     * calling toString(). Setting it to an invalid URL throws a TypeError.
     *
     */
    href: string
    /**
     * Gets the read-only serialization of the URL's origin.
     *
     */
    readonly origin: string
    /**
     * 
     * Gets and sets the password portion of the URL.
     * Invalid URL characters in the assigned value are percent-encoded.
     *
     */
    password: string
    /**
     * 
     * Gets and sets the path portion of the URL.
     * Invalid URL characters in the assigned value are percent-encoded.
     *
     */
    pathname: string
    /**
     * 
     * Gets and sets the port portion of the URL as a string.
     * Values outside the range 0–65535 are ignored. Setting to the protocol's
     * default port results in an empty string. Non-integer numbers are truncated.
     *
     */
    port: string
    /**
     * 
     * Gets and sets the protocol portion of the URL (including the trailing colon).
     * Invalid protocol values are ignored.
     *
     */
    protocol: string
    /**
     * 
     * Gets and sets the serialized query portion of the URL.
     * Invalid URL characters in the assigned value are percent-encoded.
     *
     */
    search: string
    /**
     * 
     * Gets the URLSearchParams object representing the query parameters of the URL.
     * This property is read-only but the returned object can mutate the URL instance.
     * Use care when mixing the search setter and searchParams mutators, as they apply
     * different percent-encoding rules.
     *
     */
    readonly searchParams: URLSearchParams
    /**
     * 
     * Gets and sets the username portion of the URL.
     * Invalid URL characters in the assigned value are percent-encoded.
     *
     */
    username: string
    /**
     * 
     * Returns the serialized URL. Equivalent to reading the href property or toJSON().
     * @returns The full serialized URL string.
     *
     */
    toString(): string
    /**
     * 
     * Returns the serialized URL. Equivalent to href and toString().
     * Automatically called when the object is serialized with JSON.stringify().
     * @returns The full serialized URL string.
     *
     */
    toJSON(): string
  }
  /**
   * 
   * The URLSearchParams API provides read and write access to the query of a URL.
   * It can also be used standalone. The class is also available on the global object.
   *
   */
  export class URLSearchParams implements Iterable<[string, string]> {
    /**
     * Create a new URLSearchParams object.
     * @param init? Optional initial data.
     *
     */
    constructor (init?: URLSearchParams | string | Record<string, string | readonly string[]> | Iterable<[string, string]> | ReadonlyArray<[string, string]>)
    /**
     * Appends a new name-value pair to the query string.
     * @param name The parameter name.
     * @param value The parameter value.
     *
     */
    append(name: string, value: string): void
    /**
     * 
     * Removes name-value pairs from the query string.
     * If value is provided, only pairs matching both name and value are removed.
     * If value is omitted, all pairs whose name matches are removed.
     * @param name The parameter name to remove.
     * @param value? Optional specific value to match.
     *
     */
    delete(name: string, value?: string): void
    /**
     * 
     * Returns an ES6 Iterator over each name-value pair as a two-element [name, value] array.
     * Alias for urlSearchParams[Symbol.iterator]().
     * @returns Iterator of [name, value] tuples.
     *
     */
    entries(): URLSearchParamsIterator<[string, string]>
    /**
     * 
     * Iterates over each name-value pair in the query and invokes the given function.
     * @param fn Callback invoked for each name-value pair.
     * @param thisArg? Value to use as this when calling fn.
     *
     */
    forEach(fn: (value: string, name: string, searchParams: URLSearchParams) => void, thisArg?: any): void
    /**
     * 
     * Returns the value of the first name-value pair whose name matches.
     * Returns null if no such pair exists.
     * @param name The parameter name to look up.
     * @returns The first matching value or null.
     *
     */
    get(name: string): string | null
    /**
     * 
     * Returns all values of name-value pairs whose name matches as an array.
     * Returns an empty array if no such pairs exist.
     * @param name The parameter name to look up.
     * @returns Array of all matching values.
     *
     */
    getAll(name: string): string[]
    /**
     * 
     * Checks whether the URLSearchParams object contains key-value pair(s).
     * If value is provided, returns true only when both name and value match.
     * If value is omitted, returns true if at least one pair with that name exists.
     * @param name The parameter name to check.
     * @param value? Optional specific value to check.
     * @returns Whether a matching pair exists.
     *
     */
    has(name: string, value?: string): boolean
    /**
     * Returns an ES6 Iterator over the names of each name-value pair.
     * @returns Iterator of parameter names.
     *
     */
    keys(): URLSearchParamsIterator<string>
    /**
     * 
     * Sets the value associated with name to value.
     * If pre-existing pairs share that name, the first is updated and the rest are removed.
     * If no pair exists, a new one is appended.
     * @param name The parameter name.
     * @param value The parameter value.
     *
     */
    set(name: string, value: string): void
    /**
     * The total number of parameter entries.
     *
     */
    readonly size: number
    /**
     * 
     * Sorts all existing name-value pairs in-place by their names using a stable
     * sorting algorithm, so relative order between pairs with the same name is preserved.
     * Useful for increasing cache hits.
     *
     */
    sort(): void
    /**
     * 
     * Returns the search parameters serialized as a string with characters
     * percent-encoded where necessary.
     * @returns The serialized query string.
     *
     */
    toString(): string
    /**
     * Returns an ES6 Iterator over the values of each name-value pair.
     * @returns Iterator of parameter values.
     *
     */
    values(): URLSearchParamsIterator<string>
    [Symbol.iterator](): URLSearchParamsIterator<[string, string]>
  }
  /**
   * 
   * A value that can be coerced into T, either directly, via `valueOf()`, or via
   * a string `Symbol.toPrimitive` hint. Used by several `Buffer.from` overloads.
   * 
   *
   */
  type WithImplicitCoercion<T> = T | { valueOf(): T } | (T extends string ? { [Symbol.toPrimitive](hint: "string"): T } : never);
  /**
   * 
   * The set of character encodings supported by Buffer operations in this runtime.
   * Note: ascii, utf16le, ucs2, latin1, and binary are not supported.
   *
   */
  type BufferEncoding = "utf8" | "utf-8" | "base64" | "base64url" | "hex";
}
