// Copyright 2010 Simon Lipp.
// Distributed under a BSD-like license. See COPYING for more
// details

// This package is used for writing mails to a maildir, according to
// the specification located at http://www.courier-mta.org/maildir.html
package maildir

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	paths "path"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

var (
	// a modified form of base64-encoding (no padding with "=" and "," instead of ".")
	maildirBase64 = base64.NewEncoding("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+,")
	// counter is used to generate unique file names
	counter chan uint
	// counterInit ensures that counter is initialized only onced
	counterInit sync.Once
	// pid is used to generate unique file names
	pid = os.Getpid()
)

// Represent a folder in a maildir. The root folder is usually the Inbox.
type Maildir struct {
	// The root path ends with a /, others don't, so we can have
	// the child of a maildir just with path + "." + encodedChildName.
	Path     string
	perm     os.FileMode
	uid, gid int
}

const DoNotSetOwner = -1

// Default file perms for files. For directories u+x will be added
const DefaultFilePerm = 0600

func newWithRawPath(path string, create bool, perm os.FileMode, uid, gid int) (m *Maildir, err error) {
	// start counter if needed, preventing race condition
	counterInit.Do(func() {
		counter = make(chan uint)
		go (func() {
			for i := uint(0); true; i++ {
				counter <- i
			}
		})()
	})
	// Directories need an extra x permission so they can be accessed
	// Set an x for every r in user/group/other
	dirPerm := os.FileMode(perm | ((perm & 0444) >> 2))

	// Create if needed
	if _, err := os.Stat(path); create && err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(path, dirPerm); err != nil {
			return nil, err
		}
		if err = changeOwner(path, uid, gid); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if create {
		if err := createSubFolders(path, dirPerm, uid, gid); err != nil {
			return nil, err
		}
	}

	return &Maildir{path, perm, uid, gid}, nil
}

// createSubFolders creates the tmp/, cur/ and new/ sub-folders folders
func createSubFolders(path string, dirPerm os.FileMode, uid, gid int) error {
	// check that the sub-folders exist, if not create them
	for _, subdir := range []string{"tmp", "cur", "new"} {
		ps := paths.Join(path, subdir)
		if _, err := os.Stat(ps); os.IsNotExist(err) {
			if err := os.Mkdir(ps, dirPerm); err != nil {
				return err
			}
			if err := changeOwner(ps, uid, gid); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	return nil
}

// Open a maildir. If create is true and the maildir does not exist, create it.
func New(path string, create bool) (m *Maildir, err error) {
	path = normalizePath(path)
	return newWithRawPath(path, create, DefaultFilePerm, DoNotSetOwner, DoNotSetOwner)
}

// Same as New, but ability to control permissions
// perm is an octal used for os.Chmod and what will be used for files
// for directories, an additional chmod u+x will be applied.
// uid and gid are for os.Chown, pass DoNotSetOwner constant to ignore.
func NewWithPerm(path string, create bool, perm os.FileMode, uid, gid int) (m *Maildir, err error) {
	path = normalizePath(path)
	return newWithRawPath(path, create, perm, uid, gid)
}

// normalizePath ensures that path is not empty and ends with a /
func normalizePath(p string) string {
	if len(p) == 0 {
		p = "." + string(os.PathSeparator)
	} else if !os.IsPathSeparator(p[len(p)-1]) {
		p += string(os.PathSeparator)
	}
	return p
}

// Get a subfolder of the current folder. If create is true and the folder does not
// exist, create it.
func (m *Maildir) Child(name string, create bool) (*Maildir, error) {
	encodedPath := m.encodeName(name)
	return newWithRawPath(encodedPath.String(), create, m.perm, m.uid, m.gid)
}

// encodeName encodes non valid characters according to mailbox folder nameing spec
func (m *Maildir) encodeName(name string) *bytes.Buffer {
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
	return encodedPath
}

// Write a mail to the maildir folder. The data is not encoded or compressed in any way.
func (m *Maildir) CreateMail(data io.Reader) (filename string, err error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}

	basename := fmt.Sprintf("%v.M%vP%v_%v.%v", time.Now().Unix(), time.Now().Nanosecond()/1000, pid, <-counter, hostname)
	tmpname := paths.Join(m.Path, "tmp", basename)
	file, err := os.OpenFile(tmpname, os.O_RDWR|os.O_CREATE|os.O_TRUNC, m.perm)
	if err != nil {
		return "", err
	}
	defer file.Close()
	size, err := io.Copy(file, data)
	if err != nil {
		os.Remove(tmpname)
		return "", err
	}
	file.Sync()

	newname := paths.Join(m.Path, "new", fmt.Sprintf("%v,S=%v", basename, size))
	err = os.Rename(tmpname, newname)
	if err != nil {
		os.Remove(tmpname)
		return "", err
	}

	err = changeOwner(newname, m.gid, m.uid)
	if err != nil {
		// don't want to leave files with bad permissions
		os.Remove(newname)
		return "", err
	}

	return newname, nil
}

// changeOwner changes the owner of the path.
// No changes will be made if uid or guid are set to const DoNotSetOwner
func changeOwner(path string, uid, gid int) error {
	if uid == DoNotSetOwner || gid == DoNotSetOwner {
		return nil
	}
	return os.Chown(path, uid, gid)
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
	utf16data := utf16.Encode([]rune(s))
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
