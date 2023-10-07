package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"gopkg.in/gomail.v2"
)

var mutex = &sync.Mutex{}
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var clients = make(map[*websocket.Conn]bool)

func main() {
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/upload", uploadHandler)
	http.HandleFunc("/ws", wsHandler)
	http.HandleFunc("/download/", downloadHandler)

	fmt.Println("Server running on http://localhost:8080")
	log.Fatal(http.ListenAndServe("0.0.0.0:8080", nil))
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Got a file upload request")
	file, _, err := r.FormFile("files")
	if err != nil {
		http.Error(w, "Unable to read file", http.StatusBadRequest)
		log.Println("File read error:", err)
		return
	}
	defer file.Close()

	tempFile, err := ioutil.TempFile("./", "upload-*.tmp")
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		log.Println("Temp file error:", err)
		return
	}

	fileBytes, _ := ioutil.ReadAll(file)
	tempFile.Write(fileBytes)
	fileName := filepath.Base(tempFile.Name())
	tempFile.Close()

	language := getFormValueOrDefault(r, "language", "Spanish")
	model := getFormValueOrDefault(r, "model", "small")

	cmdStr := fmt.Sprintf("whisper %s --language %s --model %s --output_format txt", fileName, language, model)
	fmt.Println(cmdStr)

	log.Println("Got the file. About to transcribe...")
	go executeCommand(cmdStr, fileName)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "File uploaded successfully"})
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("WebSocket connected")
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket Upgrade Error: %v", err)
		return
	}
	mutex.Lock()
	clients[conn] = true
	mutex.Unlock()
	log.Println("Added client, clients are now: ", clients)
	defer func() {
		log.Println("WebSocket disconnected")
		mutex.Lock()
		delete(clients, conn)
		mutex.Unlock()
		conn.Close()
	}()
	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			log.Println("WebSocket read error: ", err)
			return
		}
		// Echo back for now; this can be changed to whatever processing you need
		if err := conn.WriteMessage(messageType, p); err != nil {
			log.Println("WebSocket write error: ", err)
			return
		}
	}
}

func sendFileByEmail(filePath string) {
	// Establece las credenciales de correo electrónico
	smtpServer := "smtp.gmail.com"
	emailSender := "gonzalohll@gmail.com"
	emailPassword := "ighn xupo nsmq rnmk"
	emailRecipients := []string{"lourdescorrea9135@gmail.com", "gonzalohll@gmail.com"}

	// Crear un mensaje de correo electrónico
	msg := gomail.NewMessage()
	msg.SetHeader("From", emailSender)
	for _, recipient := range emailRecipients {
		msg.SetHeader("To", recipient)
	}
	msg.SetHeader("Subject", "Transcripción de audio")
	msg.SetBody("text/plain", "Este es el archivo generado...")

	// Adjunta el archivo
	msg.Attach(filePath)

	// Inicia el cliente SMTP
	d := gomail.NewDialer(smtpServer, 587, emailSender, emailPassword)

	// Envía el correo electrónico
	if err := d.DialAndSend(msg); err != nil {
		log.Println(err)
	}

}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	fileName := filepath.Base(r.URL.Path)
	filePath := "./" + fileName

	http.ServeFile(w, r, filePath)

	// Envía el archivo por correo electrónico
	sendFileByEmail(filePath)

	// Delete the file
	deleteOldFiles()
}

func deleteOldFiles() {
	// Get current time
	now := time.Now()

	// Path to your folder
	dir := "./"

	// Read directory for .txt and .tmp files
	filePatterns := []string{"*.txt", "*.tmp"}
	for _, pattern := range filePatterns {
		files, _ := filepath.Glob(filepath.Join(dir, pattern))

		for _, f := range files {
			// Get file info
			fileInfo, err := os.Stat(f)
			if err != nil {
				log.Println(err)
				continue
			}

			// Calculate time difference
			if now.Sub(fileInfo.ModTime()) > (24 * time.Hour) {
				// Delete file
				err := os.Remove(f)
				if err != nil {
					log.Println(err)
				} else {
					log.Printf("Deleted: %s\n", f)
				}
			}
		}
	}
}

func executeCommand(commandStr, fileName string) {
	// Split command string into command and arguments
	cmdFields := strings.Fields(commandStr)

	// Create a new context with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)

	defer cancel()

	// Create the command with the provided arguments
	cmd := exec.CommandContext(ctx, cmdFields[0], cmdFields[1:]...)

	// Capture stdout and stderr
	var out strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	// Execute the command
	err := cmd.Run()

	// Check for errors in execution
	if err != nil {
		log.Printf("Error: %s\n", stderr.String())
		return
	}

	// Here, we simulate that the command has generated a text file
	// In real-world applications, you'd want to capture this from the command output
	txtFileName := fileName + ".txt"
	err = ioutil.WriteFile(txtFileName, []byte(out.String()), 0644)
	if err != nil {
		log.Printf("Error writing to file: %s\n", err)
		return
	}

	log.Println("Done, should notify client now...")
	// Notify clients that the txt file is ready for download
	notifyClients(txtFileName)
	log.Println("Should have notified clients by now.")
}

func notifyClients(fileName string) {
	mutex.Lock()
	log.Println("Clients: ", clients)
	for client := range clients {
		log.Println("This should show when notifying frontend that the file is ready.")
		err := client.WriteMessage(1, []byte(fileName))
		if err != nil {
			log.Printf("WebSocket error: %v", err)
			client.Close()
			delete(clients, client)
		}
	}
	mutex.Unlock()
	log.Println("Done.")
}

func getFormValueOrDefault(r *http.Request, key, defaultValue string) string {
	if val := r.FormValue(key); val != "" {
		return val
	}
	return defaultValue
}
