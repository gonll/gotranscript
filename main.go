package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	if _, err := os.Stat("src/files"); os.IsNotExist(err) {
		os.MkdirAll("src/files", 0755)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "src/uploader.html")
	})

	http.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		r.ParseMultipartForm(10 << 20) // 10 MB limit
		file, _, err := r.FormFile("files")
		if err != nil {
			log.Fatal(err)
			return
		}
		defer file.Close()

		tempFile, err := ioutil.TempFile("./", "upload-*.tmp")
		if err != nil {
			log.Fatal(err)
			return
		}
		defer tempFile.Close()

		fileBytes, _ := ioutil.ReadAll(file)
		tempFile.Write(fileBytes)
		fileName := filepath.Base(tempFile.Name())

		//Handle parameters and command variables
		language := ""
		if r.FormValue("language") == "" {
			language = "--language Spanish"
		} else {
			language = "--language " + r.FormValue("language")
		}

		model := ""
		if r.FormValue("model") == "" {
			model = "--model small"
		} else {
			model = "--model " + r.FormValue("model")
		}

		translate := ""
		if r.FormValue("translate") == "" {
			translate = ""
		} else {
			translate = "--translate " + r.FormValue("translate")
		}

		// Combine into whisper command arguments
		cmdStr := fmt.Sprintf("whisper %s %s %s %s", fileName, language, model, translate)
		fmt.Println(cmdStr)
		// Run whisper command
		cmd := exec.Command("sh", "-c", cmdStr)
		_, err = cmd.Output()
		if err != nil {
			fmt.Fprintf(w, "Failed to execute command: %s", err)
			return
		}

		// Assume whisper produces a .txt file
		txtFileName := strings.Replace(tempFile.Name(), ".tmp", ".txt", 1)

		// Serve txt file
		http.ServeFile(w, r, txtFileName)

		// Manually close tempFile
		tempFile.Close()

		// Files clean up
		baseFileName := strings.TrimSuffix(tempFile.Name(), ".tmp")
		extensions := []string{"txt", "json", "srt", "tmp", "tsv", "vtt"}

		for _, ext := range extensions {
			fileName := fmt.Sprintf("%s.%s", baseFileName, ext)
			err := os.Remove(fileName)
			if err != nil {
				fmt.Printf("Failed to delete %s: %s\n", fileName, err)
			}
		}
	})

	fmt.Println("Server running on http://localhost:3000")
	log.Fatal(http.ListenAndServe(":3001", nil))
}
