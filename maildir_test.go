package maildir

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	paths "path"
	"strings"
	"testing"
)

type encodingTestData struct {
	decoded, encoded string
}

var encodingTests = []encodingTestData{
	{"&2[foo]", "&-2[foo]"},                               // Folder name starting with a special character
	{"foo&", "foo&-"},                                     // Folder name ending with a special character
	{"A./B", "A&AC4ALw-B"},                                // "." and "/" are special
	{"Lesson:日本語", "Lesson:&ZeVnLIqe-"},                   // long sequence of characters
	{"Résumé&Écritures", "R&AOk-sum&AOk-&-&AMk-critures"}, // "&" in the middle of a sequence of special characters
	{"Hello world", "Hello world"},                        // spaces are not encoded
}

func TestCreate(t *testing.T) {
	if err := os.RemoveAll("_obj/Maildir"); err != nil {
		panic(fmt.Sprintf("Can't remove old test data: %v", err))
	}

	// Opening non-existing maildir
	md, err := New("_obj/Maildir", false)
	if md != nil {
		t.Error("I shouldn't be able to open a non-existent maildir")
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
		t.Error("No error, but nil maildir when creating a maildir")
		return
	}

	// Chek that cur/, tmp/ and new/ have been created
	for _, subdir := range []string{"cur", "tmp", "new"} {
		fi, err := os.Stat("_obj/Maildir/" + subdir)
		if err != nil {
			t.Errorf("Can't open %v of maildir _obj/Maildir: %v", subdir, err)
			continue
		}
		if !fi.IsDir() {
			t.Errorf("%v of maildir _obj/Maildir is not a directory", subdir)
			continue
		}
	}
}

func TestCreateWithPerms(t *testing.T) {
	if err := os.RemoveAll("_obj/Maildir"); err != nil {
		panic(fmt.Sprintf("Can't remove old test data: %v", err))
	}

	// Creating new maildir
	md, err := NewWithPerm("_obj/Maildir", true, 0644, DoNotSetOwner, DoNotSetOwner)
	defer os.RemoveAll("_obj/Maildir")
	if err != nil {
		t.Errorf("Error while creating maildir: %v", err)
		return
	}
	if md == nil {
		t.Error("No error, but nil maildir when creating a maildir")
		return
	}
	// check correct permissions
	if fi, statErr := os.Stat("_obj/Maildir"); statErr != nil {
		t.Error("could not stat _obj/Maildir", statErr)
	} else if perm := fi.Mode().Perm(); perm != 0755 {
		t.Errorf("expected permissions of _obj/Maildir 0755, but got %o", perm)
	}

	// Chek that cur/, tmp/ and new/ have correct perms
	for _, subdir := range []string{"cur", "tmp", "new"} {
		fi, err := os.Stat("_obj/Maildir/" + subdir)
		if err != nil {
			t.Errorf("Can't open %v of maildir _obj/Maildir: %v", subdir, err)
			continue
		}
		if !fi.IsDir() {
			t.Errorf("%v of maildir _obj/Maildir is not a directory", subdir)
			continue
		}

		// for every r permission for u,g,o it should add an x permission
		if perm := fi.Mode().Perm(); perm != 0755 {
			t.Errorf("expected permissions %v of maildir of _obj/Maildir 0755, but got %o", subdir, perm)
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
		if child.Path != "_obj/Maildir/."+testData.encoded {
			t.Logf("Sub-maildir %v has an invalid path", testData.decoded)
			t.Logf(" Expected result: %s", "_obj/Maildir/."+testData.encoded)
			t.Logf("   Actual result: %s", child.Path)
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

	if child.Path != "_obj/Maildir/.foo.bar" {
		t.Logf("Sub-maildir %v has an invalid path", "foo/bar")
		t.Logf(" Expected result: %s", "_obj/Maildir/.foo.bar")
		t.Logf("   Actual result: %s", child.Path)
		t.Fail()
	}
}

func readdirnames(dir string) ([]string, error) {
	d, err := os.Open(dir)
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

func TestWritePerms(t *testing.T) {
	if err := os.RemoveAll("_obj/Maildir"); err != nil {
		panic(fmt.Sprintf("Can't remove old test data: %v", err))
	}

	maildir, err := NewWithPerm("_obj/Maildir", true, 0644, DoNotSetOwner, DoNotSetOwner)
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
	// fullName should have our pid
	if strings.Index(fullName, fmt.Sprintf("%d_", os.Getpid())) == -1 {
		t.Error("fullName does not contain the pid, it was:", fullName)
	}

	// check perms
	if fi, err := os.Stat(fullName); err != nil {
		t.Error("could not stat", fullName)
	} else {
		if perm := fi.Mode().Perm(); perm != 0644 {
			t.Errorf("expected permissions %v  600, 0644 got %o", fullName, perm)
		}
	}
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

	f, err := os.Open(fullName)
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

// For folder with no sub folders, it should create the sub-folders
func TestFolderWithNoSubFolders(t *testing.T) {
	dir := "_obj/Maildir"
	if err := os.RemoveAll(dir); err != nil {
		panic(fmt.Sprintf("Can't remove old test data: %v", err))
	}

	// create a maildir folder without any sub-dirs
	dirPerm := os.FileMode(DefaultFilePerm | ((DefaultFilePerm & 0444) >> 2))
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		t.Errorf("Can't create maildir: %v", err)
		return
	}
	// this will create the sub-folders
	maildir, err := New(dir, true)
	if maildir == nil {
		t.Errorf("Can't create maildir: %v", err)
		return
	}
	// sub-dirs should be there
	for _, subdir := range []string{"tmp", "cur", "new"} {
		ps := paths.Join(dir, subdir)
		if _, err = os.Stat(ps); os.IsNotExist(err) {
			t.Error("sub folder does not exist. Expected ", ps)
		}
	}
	defer os.RemoveAll("_obj/Maildir")
}
