package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

// Get Git repository path from ENV variable, fallback if not set
var gitRepoPath = getRepoPath()

func getRepoPath() string {
	if path, exists := os.LookupEnv("GIT_REPO_PATH"); exists {
		return path
	}

	return "/Users/suman/Desktop/superview/accounting" // Change this to a reasonable fallback
}

// GitCommand represents a request to run a git command.
type GitCommand struct {
	Command []string `json:"command"`
}

// runGit executes a git command inside the Git repo directory.
func runGit(command ...string) (string, error) {
	cmd := exec.Command("git", command...)
	cmd.Dir = gitRepoPath // Enforce the working directory

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return stderr.String(), err
	}
	return out.String(), nil
}

// rootHandler: Serve the main page with Git status and a command input form
func rootHandler(w http.ResponseWriter, r *http.Request) {
	gitStatus, err := runGit("status")
	if err != nil {
		gitStatus = fmt.Sprintf("Error getting git status: %s", err.Error())
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Git Server</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        textarea { width: 100%%; height: 100px; }
        pre { background: #f4f4f4; padding: 10px; }
    </style>
</head>
<body>
    <h2>Git Status</h2>
    <pre>%s</pre>

    <h2>Run Git Command</h2>
    <form id="gitForm">
        <input type="text" id="command" placeholder="Enter git command (e.g., log --oneline)" style="width: 80%%;">
        <button type="submit">Run</button>
    </form>
    <h3>Output:</h3>
    <pre id="output"></pre>

    <script>
        document.getElementById("gitForm").onsubmit = async function(event) {
            event.preventDefault();
            let command = document.getElementById("command").value.split(" ");
            let response = await fetch("/git/run", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ command })
            });
            let result = await response.text();
            document.getElementById("output").innerText = result;
        };
    </script>
</body>
</html>`, gitStatus)

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// gitCommandHandler: Executes Git commands via POST request
func gitCommandHandler(w http.ResponseWriter, r *http.Request) {
	var cmd GitCommand
	err := json.NewDecoder(r.Body).Decode(&cmd)
	if err != nil || len(cmd.Command) == 0 {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Validate allowed Git commands
	allowedCommands := map[string]bool{
		"status":   true,
		"log":      true,
		"diff":     true,
		"pull":     true,
		"push":     true,
		"add":      true,
		"commit":   true,
		"checkout": true,
		"branch":   true,
	}

	if !allowedCommands[cmd.Command[0]] {
		http.Error(w, "Forbidden command", http.StatusForbidden)
		return
	}

	output, err := runGit(cmd.Command...)
	if err != nil {
		http.Error(w, output, http.StatusInternalServerError)
		return
	}

	w.Write([]byte(output))
}

// main starts the server
func main() {
	// Ensure the Git repo directory exists
	absPath, err := filepath.Abs(gitRepoPath)
	if err != nil {
		log.Fatalf("Invalid repo path: %v", err)
	}
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		log.Fatalf("Git repo directory does not exist: %s", absPath)
	}

	log.Println("Git server running on :7001 in directory:", absPath)
	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/git/run", gitCommandHandler)
	log.Fatal(http.ListenAndServe(":7001", nil))
}
