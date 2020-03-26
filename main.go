package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"text/template"
)

type MultilBlockFile struct {
	FileName    string
	Size        int64
	BlockSize   int64
	TotalBlocks int
	Index       int
	Bufs        []byte
	BreakError  bool
}

func fileIsExist(f string) bool {
	_, err := os.Stat(f)
	return err == nil || os.IsExist(err)
}

func lockFile(f *os.File) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		return fmt.Errorf("get flock failed. err: %s", err)
	}

	return nil
}

func unlockFile(f *os.File) error {
	defer f.Close()
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

func singleFileSave(mbf *MultilBlockFile) error {

	dir, _ := filepath.Abs(filepath.Dir(os.Args[0]))
	filePath := path.Join(dir, "tmp", mbf.FileName)

	offset := int64(mbf.Index) * mbf.BlockSize

	fmt.Println(">>> Single file save ---------------------")
	fmt.Printf("Save file: %s \n", filePath)
	fmt.Printf("File offset: %d \n", offset)

	var f *os.File
	var needTruncate bool = false
	if !fileIsExist(filePath) {
		needTruncate = true
	}

	f, _ = os.OpenFile(filePath, syscall.O_CREAT|syscall.O_WRONLY, 0777)

	err := lockFile(f)
	if err != nil {
		if mbf.BreakError {
			log.Fatalf("get flock failed. err: %s", err)
		} else {
			return err
		}
	}

	if needTruncate {
		f.Truncate(mbf.Size)
	}

	f.WriteAt(mbf.Bufs, offset)

	unlockFile(f)

	return nil
}

func multilBlocksSave(mbf *MultilBlockFile) error {
	dir, _ := filepath.Abs(filepath.Dir(os.Args[0]))
	tmpFolderPath := path.Join(dir, "tmp")
	tmpFileName := fmt.Sprintf("%s.%d", mbf.FileName, mbf.Index)
	fileBlockPath := path.Join(tmpFolderPath, tmpFileName)

	f, _ := os.OpenFile(fileBlockPath, syscall.O_CREAT|syscall.O_WRONLY|syscall.O_TRUNC, 0777)
	defer f.Close()

	f.Write(mbf.Bufs)
	f.Close()

	re := regexp.MustCompile(`(?i:^` + mbf.FileName + `).\d$`)

	files, _ := ioutil.ReadDir(tmpFolderPath)
	matchFiles := make(map[string]bool)

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		fname := file.Name()
		if re.MatchString(fname) {
			matchFiles[fname] = true
		}
	}

	if len(matchFiles) >= mbf.TotalBlocks {
		lastFile, _ := os.OpenFile(path.Join(tmpFolderPath, mbf.FileName), syscall.O_CREAT|syscall.O_WRONLY, 0777)
		lockFile(lastFile)

		lastFile.Truncate(mbf.Size)

		for name := range matchFiles {
			tmpPath := path.Join(tmpFolderPath, name)

			idxStr := name[strings.LastIndex(name, ".")+1:]
			idx, _ := strconv.ParseInt(idxStr, 10, 32)

			fmt.Printf("Match file: %s index: %d \n", name, idx)

			data, _ := ioutil.ReadFile(tmpPath)

			lastFile.WriteAt(data, idx*mbf.BlockSize)

			os.Remove(tmpPath)
		}
		unlockFile(lastFile)
	}

	return nil
}

func indexHandle(w http.ResponseWriter, r *http.Request) {
	tmp, _ := template.ParseFiles("./static/index.html")
	tmp.Execute(w, "Index")
}

func uploadHandle(w http.ResponseWriter, r *http.Request) {

	var mbf MultilBlockFile
	mbf.FileName = r.FormValue("file_name")
	mbf.Size, _ = strconv.ParseInt(r.FormValue("file_size"), 10, 64)
	mbf.BlockSize, _ = strconv.ParseInt(r.FormValue("block_size"), 10, 64)
	mbf.BreakError, _ = strconv.ParseBool(r.FormValue("break_error"))

	var i int64
	i, _ = strconv.ParseInt(r.FormValue("total_blocks"), 10, 32)
	mbf.TotalBlocks = int(i)

	i, _ = strconv.ParseInt(r.FormValue("index"), 10, 32)
	mbf.Index = int(i)

	d, _, _ := r.FormFile("data")
	mbf.Bufs, _ = ioutil.ReadAll(d)

	fmt.Printf(">>> Upload --------------------- \n")
	fmt.Printf("File name: %s \n", mbf.FileName)
	fmt.Printf("Size: %d \n", mbf.Size)
	fmt.Printf("Block size: %d \n", mbf.BlockSize)
	fmt.Printf("Total blocks: %d \n", mbf.TotalBlocks)
	fmt.Printf("Index: %d \n", mbf.Index)
	fmt.Println("Bufs len:", len(mbf.Bufs))

	multilBlockFile, _ := strconv.ParseBool(r.FormValue("multil_block"))

	var err error
	if multilBlockFile {
		err = multilBlocksSave(&mbf)
	} else {
		err = singleFileSave(&mbf)
	}

	if !mbf.BreakError && err != nil {
		w.WriteHeader(400)
		fmt.Fprintf(w, fmt.Sprintf("%s", err))
		return
	}

	fmt.Fprintf(w, "ok")
}

func main() {
	println("Listen on 8080")

	http.HandleFunc("/", indexHandle)
	http.HandleFunc("/upload", uploadHandle)

	log.Fatal(http.ListenAndServe(":8080", nil))
}
