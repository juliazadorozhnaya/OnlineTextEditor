package file

import (
	"io/ioutil"
)

type File struct {
	Name string
}

func New(name string) *File {
	return &File{Name: name}
}

func (f *File) Read() (string, error) {
	bytes, err := ioutil.ReadFile(f.Name)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (f *File) Write(content []byte) error {
	return ioutil.WriteFile(f.Name, content, 0666)
}
