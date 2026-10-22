package auth

import "bytes"
import "io"

// newBodyReader:Parse*ResponseBody 收 io.Reader;小 helper 避免每处 bytes导入。
func newBodyReader(b []byte) io.Reader { return bytes.NewReader(b) }
