# PACKAGE

package maildir

This package is used for writing mails to a maildir, according to
the specification located at http://www.courier-mta.org/maildir.html


# TYPES

       type Maildir struct {
           // The root path ends with a /, others don't, so we can have 
           // the child of a maildir just with path + "." + encodedChildName.
           Path string
       }
Represent a folder in a maildir. The root folder is usually the Inbox.

`func New(path string, create bool) (m *Maildir, err os.Error)`

Same as the New function, but with the ability to control permissions.
perm is an octal used for os.Chmod and what will be applied on files.
For directories only: an additional chmod +x will be added
for every r permission in user/group/other to make the directory
accessible.

uid and gid are for os.Chown, pass DoNotSetOwner constant to ignore.

`NewWithPerm(path string, create bool, perm os.FileMode, uid, gid int) (m *Maildir, err error)`

Open a maildir. If create is true and the maildir does not exist, create it.

`func (m *Maildir) Child(name string, create bool) (*Maildir, os.Error)`

Get a subfolder of the current folder. If create is true and the folder does not
exist, create it.

`func (m *Maildir) CreateMail(data io.Reader) (filename string, err os.Error)`

Write a mail to the maildir folder. The data is not encoded or compressed in any way.

# Fuzz test

There's a fuzz test for the encodeName() function. 

To prepare, run:

`$ go-fuzz-build github.com/flashmob/go-maildir`

it will take a while...

then start like this:

`$ go-fuzz -bin=maildir-fuzz.zip -workdir=workdir -procs=8`

where procs is the number of CPU cores to use. The test
will run in parallel and it will produce some stats.

Read more about these stats here https://github.com/dvyukov/go-fuzz

After running the test, go to the ./wokrdir and inspect your 
crashes & commit any new corpus