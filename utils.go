package main

import (
	"bytes"
	"io/ioutil"
	"os"
)

func FileExistBool(filename string) bool {
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return true
}

func FolderExistBool(folder string) bool {
	_, err := ioutil.ReadDir(folder)
	if err != nil {
		return false
	}
	return true
}
func FileReWriteByte(filename string, b []byte) error {
	err := ioutil.WriteFile(filename, b, 0644)
	if err != nil {
		return err
	}
	return nil
}

//FileCompareByteBool
/*
function compare file content and byte content

return true if b equal filename content
return false if b not equal filename content or error
*/
func FileCompareByteBool(filename string, b []byte) (bool, error) {
	dat, err := ioutil.ReadFile(filename)
	if err != nil {
		return false, err
	}
	res := bytes.Compare(dat, b)
	if res == 0 {
		return true, nil
	}
	return false, nil
}
