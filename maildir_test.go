package maildir

import (
	"testing"
	"fmt"
	"os"
	"io/ioutil"
	"strings"
	"bytes"
	"path"
)

type encodingTestData struct {
	decoded, encoded string
}

var encodingTests = []encodingTestData{
	{"&2[foo]", "&-2[foo]"},                               // Folder name starting with a special character
	{"foo&", "foo&-"},                                     // Folder name ending with a special character
	{"A./B", "A&AC4ALw-B"},                                // "." and "/" are special
	{"Lesson:日本語", "Lesson:&ZeVnLIqe-"},                // long sequence of characters
	{"Résumé&Écritures", "R&AOk-sum&AOk-&-&AMk-critures"}, // "&" in the middle of a sequence of special characters
}

func TestCreate(t *testing.T) {
	if err := os.RemoveAll("_obj/Maildir"); err != nil {
		panic(fmt.Sprintf("Can't remove old test data: %v", err))
	}

	// Opening non-existing maildir
	md, err := New("_obj/Maildir", false)
	if md != nil {
		t.Errorf("I shouldn't be able to open a non-existent maildir")
		return
	}

	// Creating new maildir
	md, err = New("_obj/Maildir", true)
	defer os.RemoveAll("_obj/Maildir")
	if err != nil {
		t.Errorf("Error while creating maildir: %v", err)
		return
	}
	if md == nil {
		t.Errorf("No error, but nil maildir when creating a maildir")
		return
	}

	// Chek that cur/, tmp/ and new/ have been created
	for _, subdir := range []string{"cur", "tmp", "new"} {
		fi, err := os.Stat("_obj/Maildir/" + subdir)
		if err != nil {
			t.Errorf("Can't open %v of maildir _obj/Maildir: %v", subdir, err)
			continue
		}
		if !fi.IsDirectory() {
			t.Errorf("%v of maildir _obj/Maildir is not a directory", subdir)
			continue
		}
	}
}

func TestEncode(t *testing.T) {
	if err := os.RemoveAll("_obj/Maildir"); err != nil {
		panic(fmt.Sprintf("Can't remove old test data: %v", err))
	}

	maildir, err := New("_obj/Maildir", true)
	if maildir == nil {
		t.Errorf("Can't create maildir: %v", err)
		return
	}
	defer os.RemoveAll("_obj/Maildir")

	for _, testData := range encodingTests {
		child, err := maildir.Child(testData.decoded, true)
		if err != nil {
			t.Errorf("Can't create sub-maildir %v: %v", testData.decoded, err)
			continue
		}
		if child.path != "_obj/Maildir/."+testData.encoded {
			t.Logf("Sub-maildir %v has an invalid path", testData.decoded)
			t.Logf(" Expected result: %s", "_obj/Maildir/."+testData.encoded)
			t.Logf("   Actual result: %s", child.path)
			t.Fail()
			continue
		}
	}

	// Separator between sub-maildir and sub-sub-maildir should not be encoded
	child, err := maildir.Child("foo", true)
	if err != nil {
		t.Errorf("Can't create sub-maildir foo: %v", err)
		return
	}

	child, err = child.Child("bar", true)
	if err != nil {
		t.Errorf("Can't create sub-maildir foo/bar: %v", err)
		return
	}

	if child.path != "_obj/Maildir/.foo.bar" {
		t.Logf("Sub-maildir %v has an invalid path", "foo/bar")
		t.Logf(" Expected result: %s", "_obj/Maildir/.foo.bar")
		t.Logf("   Actual result: %s", child.path)
		t.Fail()
	}
}

func readdirnames(dir string) ([]string, os.Error) {
	d, err := os.Open(dir, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}

	list, err := d.Readdirnames(-1)
	if err != nil {
		return nil, err
	}

	res := make([]string, 0, len(list))
	for _, entry := range list {
		if entry != "." && entry != ".." {
			res = append(res, entry)
		}
	}

	return res, nil
}

func TestWrite(t *testing.T) {
	if err := os.RemoveAll("_obj/Maildir"); err != nil {
		panic(fmt.Sprintf("Can't remove old test data: %v", err))
	}

	maildir, err := New("_obj/Maildir", true)
	if maildir == nil {
		t.Errorf("Can't create maildir: %v", err)
		return
	}
	defer os.RemoveAll("_obj/Maildir")

	testData := []byte("Hello, world !")

	// write a mail
	fullName, err := maildir.CreateMail(bytes.NewBuffer(testData))
	if err != nil {
		t.Errorf("Can't create mail: %v", err)
	}

	// tmp/ and cur/ must be empty
	names, err := readdirnames("_obj/Maildir/tmp")
	if err != nil {
		t.Errorf("Can't read tmp/: %v", err)
		return
	}
	if len(names) > 0 {
		t.Errorf("Expected no element in tmp/, got %v", names)
	}

	names, err = readdirnames("_obj/Maildir/cur")
	if err != nil {
		t.Errorf("Can't read cur/: %v", err)
		return
	}
	if len(names) > 0 {
		t.Errorf("Expected no element in cur/, got %v", names)
	}

	// new/ must contain only one file, which must contain the written data
	names, err = readdirnames("_obj/Maildir/new")
	if err != nil {
		t.Errorf("Can't read new/: %v", err)
		return
	}
	if len(names) != 1 {
		t.Errorf("Expected one element in new/, got %v", names)
	}

	f, err := os.Open(fullName, os.O_RDONLY, 0)
	if err != nil {
		t.Errorf("Can't open %v: %v", fullName, err)
		return
	}
	data, err := ioutil.ReadAll(f)
	if err != nil {
		t.Errorf("Can't read %v: %v", fullName, err)
		return
	}

	if bytes.Compare(data, testData) != 0 {
		t.Errorf("File contains %#v, expected %#v", string(data), string(testData))
	}

	// filename must end with ,S=(mail size)
	name := names[0]
	if !strings.HasSuffix(name, fmt.Sprintf(",S=%d", len(testData))) {
		t.Errorf("Filename %#v must end with %#v", name, fmt.Sprintf(",S=%d", len(testData)))
	}
	if path.Base(fullName) != name {
		t.Errorf("Returned name %#v does not match #%v", path.Base(fullName), name)
	}
}
