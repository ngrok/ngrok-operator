/*
MIT License

Copyright (c) 2024 ngrok, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package privatedial

import "io"

// encodeDialReq hand-encodes a DialReq{host, port} protobuf message:
// field 1 (host, string, wiretype 2) and field 2 (port, int64, wiretype 0).
// Hand-rolled rather than generated/imported: the real message
// (proto/lib/private_dial/private_dial.proto in the ngrok monorepo) has
// more fields (metadata, session_req), but the gateway only requires host
// and port to resolve and dial a target, and those two scalar fields are
// simple enough not to justify a protobuf-generated dependency for a POC.
func encodeDialReq(host string, port int) []byte {
	buf := make([]byte, 0, len(host)+16)
	buf = append(buf, 0x0a) // field 1, wiretype 2 (length-delimited)
	buf = appendUvarint(buf, uint64(len(host)))
	buf = append(buf, host...)
	buf = append(buf, 0x10) // field 2, wiretype 0 (varint)
	buf = appendUvarint(buf, uint64(port))
	return buf
}

// writeVarintFrame writes payload prefixed with its length as a base-128
// varint, matching the gateway's wire format for /dial and /session
// bodies.
func writeVarintFrame(w io.Writer, payload []byte) error {
	var lenBuf [10]byte // max bytes for a base-128 varint of a uint64
	n := putUvarint(lenBuf[:], uint64(len(payload)))
	if _, err := w.Write(lenBuf[:n]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

// skipVarintFrame reads and discards one varint-length-prefixed frame from
// r, returning its raw bytes (unparsed) for callers that want to log it.
func skipVarintFrame(r io.Reader) ([]byte, error) {
	n, err := readUvarint(r)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func appendUvarint(buf []byte, x uint64) []byte {
	for x >= 0x80 {
		buf = append(buf, byte(x)|0x80)
		x >>= 7
	}
	return append(buf, byte(x))
}

func putUvarint(buf []byte, x uint64) int {
	i := 0
	for x >= 0x80 {
		buf[i] = byte(x) | 0x80
		x >>= 7
		i++
	}
	buf[i] = byte(x)
	return i + 1
}

func readUvarint(r io.Reader) (uint64, error) {
	var x uint64
	var s uint
	b := make([]byte, 1)
	for {
		if _, err := io.ReadFull(r, b); err != nil {
			return 0, err
		}
		if b[0] < 0x80 {
			return x | uint64(b[0])<<s, nil
		}
		x |= uint64(b[0]&0x7f) << s
		s += 7
	}
}
