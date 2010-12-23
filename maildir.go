// Copyright 2010 Simon Lipp.
// Distributed under a BSD-like license. See COPYING for more
// details

// This package is used for writing mails to a maildir, according to
// the specification located at http://www.courier-mta.org/maildir.html
package maildir

import (
	"encoding/base64"
	"bytes"
	"strings"
	"sync"
	"os"
	"io"
	"fmt"
	"time"
	"utf16"
	paths "path"
)

var maildirBase64 = base64.NewEncoding("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+,")
var counter chan uint
var counterInit sync.Once

// Represent a folder in a maildir. The root folder is usually the Inbox.
type Maildir struct {
	// The root path ends with a /, others don't, so we can have 
	// the child of a maildir just with path + "." + encodedChildName.
	Path string
}

func newWithRawPath(path string, create bool) (m *Maildir, err os.Error) {
	// start counter if needed, preventing race condition
	counterInit.Do(func() {
		counter = make(chan uint)
		go (func() {
			for i := uint(0); true; i++ {
				counter <- i
			}
		})()
	})

	// Create if needed
	_, err = os.Stat(path)
	if err != nil {
		if pe, ok := err.(*os.PathError); ok && pe.Error == os.ENOENT && create {
			err = os.MkdirAll(path, 0775)
			if err != nil {
				return nil, err
			}
			for _, subdir := range []string{"tmp", "cur", "new"} {
				err = os.Mkdir(paths.Join(path, subdir), 0775)
				if err != nil {
					return nil, err
				}
			}
		} else {
			return nil, err
		}
	}

	return &Maildir{path}, nil
}

// Open a maildir. If create is true and the maildir does not exist, create it.
func New(path string, create bool) (m *Maildir, err os.Error) {
	// Ensure that path is not empty and ends with a /
	if len(path) == 0 {
		path = "." + string(paths.DirSeps[0])
	} else if !strings.Contains(paths.DirSeps, string(path[len(path)-1])) {
		path += string(paths.DirSeps[0])
	}
	return newWithRawPath(path, create)
}

// Get a subfolder of the current folder. If create is true and the folder does not
// exist, create it.
func (m *Maildir) Child(name string, create bool) (*Maildir, os.Error) {
	var i int
	encodedPath := bytes.NewBufferString(m.Path + ".")
	for i = nextInvalidChar(name); i < len(name); i = nextInvalidChar(name) {
		encodedPath.WriteString(name[:i])
		j := nextValidChar(name[i:])
		encode(name[i:i+j], encodedPath)
		if j < len(name[i:]) {
			name = name[i+j:]
		} else {
			name = ""
		}
	}
	encodedPath.WriteString(name)
	return newWithRawPath(encodedPath.String(), create)
}

// Write a mail to the maildir folder. The data is not encoded or compressed in any way.
func (m *Maildir) CreateMail(data io.Reader) (filename string, err os.Error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}

	basename := fmt.Sprintf("%v.M%vP%v_%v.%v", time.Seconds(), time.Nanoseconds()/1000, os.Getpid(), <-counter, hostname)
	tmpname := paths.Join(m.Path, "tmp", basename)
	file, err := os.Open(tmpname, os.O_WRONLY | os.O_CREAT, 0664)
	if err != nil {
		return "", err
	}

	size, err := io.Copy(file, data)
	if err != nil {
		os.Remove(tmpname)
		return "", err
	}

	newname := paths.Join(m.Path, "new", fmt.Sprintf("%v,S=%v", basename, size))
	err = os.Rename(tmpname, newname)
	if err != nil {
		os.Remove(tmpname)
		return "", err
	}

	return newname, nil
}

// Valid (valid = has not to be escaped) chars = 
// ASCII 32-127 + "&" + "/" + "."
// We disallow 127 because the spec is ambiguous here: it allows 127 but not control characters,
// and 127 is a control character.
func isValidChar(b byte) bool {
	if b < 0x20 || b >= 127 {
		return false
	}
	if b == byte('.') || b == byte('/') || b == byte('&') {
		return false
	}
	return true
}

func nextInvalidChar(s string) int {
	for i := 0; i < len(s); i++ {
		if !isValidChar(s[i]) {
			return i
		}
	}
	return len(s)
}

func nextValidChar(s string) int {
	for i := 0; i < len(s); i++ {
		if isValidChar(s[i]) {
			return i
		}
	}
	return len(s)
}

// s is a string of invalid chars, without any "&"
// An encoded sequence is composed of (Python-like pseudo-code):
// "&" + base64(rawSequence.encode('utf-16-be')).strip('=') + "-"
func encodeSequence(s string, buf *bytes.Buffer) {
	utf16data := utf16.Encode([]int(s))
	utf16be := make([]byte, len(utf16data)*2)
	for i := 0; i < len(utf16data); i++ {
		utf16be[i*2] = byte(utf16data[i] >> 8)
		utf16be[i*2+1] = byte(utf16data[i] & 0xff)
	}
	base64data := make([]byte, maildirBase64.EncodedLen(len(utf16be))+2)
	maildirBase64.Encode(base64data[1:], utf16be)
	endPos := bytes.IndexByte(base64data, byte('='))
	if endPos == -1 {
		endPos = len(base64data)
	} else {
		endPos++
	}
	base64data = base64data[:endPos]
	base64data[0] = byte('&')
	base64data[len(base64data)-1] = byte('-')
	buf.Write(base64data)
}

// s in a string of invalid chars
// "&" is not encoded in a sequence, and must be encoded as "&-",
// so split s as sequences of [^&]* separated by "&" characters
func encode(s string, buf *bytes.Buffer) {
	if s[0] == byte('&') {
		buf.WriteString("&-")
		if len(s) > 1 {
			encode(s[1:], buf)
		}
	} else {
		i := strings.Index(s, "&")
		if i != -1 {
			encodeSequence(s[:i], buf)
			encode(s[i:], buf)
		} else {
			encodeSequence(s, buf)
		}
	}
}
