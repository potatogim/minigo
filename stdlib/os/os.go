package os

var Stdout *File = &File{
	id: 1,
}

var Stderr *File = &File{
	id: 2,
}

// File represents an open file descriptor.
type File struct {
	id int
}

func (f *File) Write(b []byte) (int, error) {
	var fid int = f.id
	var n int
	n = write(fid, string(b), len(b))
	return n,nil
}

func Exit(i int) {
}

func init() {

	// runtime_args is written in assembly code
	Args = runtime_args()
}

//func runtime_args() []string
