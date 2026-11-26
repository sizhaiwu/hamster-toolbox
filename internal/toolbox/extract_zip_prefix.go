package toolbox

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// 7-Zip normally searches only the first 8 MiB. Some ZIPs prepend a whole
// video. Read at most 1 MiB of footer plus fixed-size headers to locate and
// validate the ZIP; never scan/copy the full input ourselves. Keep the native
// search bounded too, and let 7-Zip perform the complete archive validation.
const maxPrefixedZIPSearch = int64(512 << 20)

func prefixedZIPTypeOptions(source string) ([]string, error) {
	f, err := os.Open(source)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var signature [4]byte
	if _, err = io.ReadFull(f, signature[:]); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil, nil
		}
		return nil, err
	}
	if signature == [4]byte{'P', 'K', 3, 4} || signature == [4]byte{'P', 'K', 5, 6} || signature == [4]byte{'P', 'K', 7, 8} {
		return nil, nil
	}
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() <= 8<<20 || !st.Mode().IsRegular() {
		return nil, nil
	}
	tailSize := min(st.Size(), int64(1<<20))
	tail := make([]byte, int(tailSize))
	if _, err = f.ReadAt(tail, st.Size()-tailSize); err != nil {
		return nil, err
	}
	le := binary.LittleEndian
	end := len(tail)
	for attempts := 0; attempts < 32; attempts++ {
		at := bytes.LastIndex(tail[:end], []byte{'P', 'K', 5, 6})
		if at < 0 {
			break
		}
		end = at
		if at+22 > len(tail) {
			continue
		}
		footer := tail[at:]
		entries := le.Uint16(footer[10:])
		// Split and ZIP64 archives retain their existing handlers. Do not
		// infer offsets from their sentinel or multi-volume fields.
		if le.Uint16(footer[4:]) != 0 || le.Uint16(footer[6:]) != 0 || entries == 0 || entries == 0xffff || le.Uint16(footer[8:]) != entries || at+22+int(le.Uint16(footer[20:])) > len(tail) {
			continue
		}
		cdSize, cdOffset := int64(le.Uint32(footer[12:])), int64(le.Uint32(footer[16:]))
		if cdSize == 0xffffffff || cdOffset == 0xffffffff || cdSize < int64(entries)*46 {
			continue
		}
		footerAt := st.Size() - tailSize + int64(at)
		prefix := footerAt - cdSize - cdOffset
		if prefix <= 8<<20 || prefix > maxPrefixedZIPSearch {
			continue
		}
		centralAt := prefix + cdOffset
		var central [46]byte
		if _, err := f.ReadAt(central[:], centralAt); err != nil || !bytes.Equal(central[:4], []byte{'P', 'K', 1, 2}) || le.Uint16(central[34:]) != 0 {
			continue
		}
		localAt := prefix + int64(le.Uint32(central[42:]))
		if localAt < prefix || localAt > centralAt-4 {
			continue
		}
		if _, err := f.ReadAt(signature[:], localAt); err == nil && signature == [4]byte{'P', 'K', 3, 4} {
			return []string{fmt.Sprintf("-tzip:s%db", prefix+1)}, nil
		}
	}
	return nil, nil
}
