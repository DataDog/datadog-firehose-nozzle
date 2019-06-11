package helper

import (
	"bytes"
	"compress/zlib"
	"io/ioutil"
)

func Decompress(src []byte) []byte {
	r, _ := zlib.NewReader(bytes.NewReader(src))
	defer r.Close()
	dst, _ := ioutil.ReadAll(r)
	return dst
}
