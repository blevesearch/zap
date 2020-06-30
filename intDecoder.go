//  Copyright (c) 2019 Couchbase, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 		http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package zap

import (
	"encoding/binary"
	"fmt"

	"github.com/bmkessler/streamvbyte"
)

type chunkedIntDecoder struct {
	startOffset     uint64
	dataStartOffset uint64
	chunkOffsets    []uint64
	chunkCounts     []uint64
	curChunkBytes   []byte
	data            []byte
	svbr            *streamVbReader
}

func newChunkedIntDecoder(buf []byte, offset uint64) *chunkedIntDecoder {
	rv := &chunkedIntDecoder{startOffset: offset, data: buf}
	var n, numChunks uint64
	var read int
	if offset == termNotEncoded {
		numChunks = 0
	} else {
		numChunks, read = binary.Uvarint(buf[offset+n : offset+n+binary.MaxVarintLen64])
	}

	n += uint64(read)
	if cap(rv.chunkOffsets) >= int(numChunks) {
		rv.chunkOffsets = rv.chunkOffsets[:int(numChunks)]
		rv.chunkCounts = rv.chunkCounts[:int(numChunks)]
	} else {
		rv.chunkOffsets = make([]uint64, int(numChunks))
		rv.chunkCounts = make([]uint64, int(numChunks))
	}
	for i := 0; i < int(numChunks); i++ {
		rv.chunkOffsets[i], read = binary.Uvarint(buf[offset+n : offset+n+binary.MaxVarintLen64])
		n += uint64(read)
		rv.chunkCounts[i], read = binary.Uvarint(buf[offset+n : offset+n+binary.MaxVarintLen64])
		n += uint64(read)
	}
	rv.dataStartOffset = offset + n
	return rv
}

func (d *chunkedIntDecoder) loadChunk(chunk int) error {
	if d.startOffset == termNotEncoded {
		d.svbr = newStreamVbReader([]byte(nil), 0) //segment.NewMemUvarintReader([]byte(nil))
		return nil
	}

	if chunk >= len(d.chunkOffsets) {
		return fmt.Errorf("tried to load freq chunk that doesn't exist %d/(%d)",
			chunk, len(d.chunkOffsets))
	}

	end, start := d.dataStartOffset, d.dataStartOffset
	s, e := readChunkBoundary(chunk, d.chunkOffsets)
	start += s
	end += e
	d.curChunkBytes = d.data[start:end]
	if d.svbr == nil {
		d.svbr = newStreamVbReader(d.curChunkBytes, d.chunkCounts[chunk])
	} else {
		d.svbr.reset(d.curChunkBytes, d.chunkCounts[chunk])
	}

	return nil
}

func (d *chunkedIntDecoder) reset() {
	d.startOffset = 0
	d.dataStartOffset = 0
	d.chunkOffsets = d.chunkOffsets[:0]
	d.chunkCounts = d.chunkCounts[:0]
	d.curChunkBytes = d.curChunkBytes[:0]
	d.data = d.data[:0]
	if d.svbr != nil {
		d.svbr.reset([]byte(nil), 0)
	}
}

func (d *chunkedIntDecoder) isNil() bool {
	return d.curChunkBytes == nil
}

func (d *chunkedIntDecoder) readUvarint() (uint64, error) {
	return d.svbr.Read()
}

func (d *chunkedIntDecoder) SkipUvarint() {
	//d.svbr.Skip()
}

func (d *chunkedIntDecoder) SkipBytes(count int) {

	d.svbr.Skip(count)
}

func (d *chunkedIntDecoder) Len() int {
	return d.svbr.Len()
}

type streamVbReader struct {
	vals []uint32
	i    int
}

func (br *streamVbReader) reset(data []byte, count uint64) {
	br.i = 0
	if len(data) == 0 {
		br.vals = br.vals[:0]
		return
	}

	if cap(br.vals) <= int(count) {
		br.vals = make([]uint32, count)
	} else {
		br.vals = br.vals[:count]
	}

	streamvbyte.DecodeUint32(br.vals, data)
}

func newStreamVbReader(data []byte, count uint64) *streamVbReader {
	br := &streamVbReader{
		vals: make([]uint32, count),
	}
	if len(data) == 0 || count == 0 {
		return br
	}

	streamvbyte.DecodeUint32(br.vals, data)
	return br
}

func (br *streamVbReader) Read() (uint64, error) {
	if len(br.vals) == 0 {
		return 0, fmt.Errorf("bytesReader Read err: EOF")
	}

	if br.i > len(br.vals)-1 {
		return 0, fmt.Errorf("out of bound err - bytesReader")
	}

	rv := br.vals[br.i]
	br.i++
	return uint64(rv), nil
}

func (br *streamVbReader) Len() int {
	n := len(br.vals) - br.i
	if n < 0 {
		return 0
	}
	return n
}

func (br *streamVbReader) Skip(skip int) error {
	if br.i+skip-1 >= len(br.vals) {
		return fmt.Errorf("skip out of bound err - bytesReader")
	}
	br.i = br.i + skip
	return nil
}
