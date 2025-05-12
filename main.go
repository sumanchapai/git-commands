package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return stderr.String(), err
	}
	return out.String(), nil
}

// Allowed Git commands (ensures security)
var allowedCommands = map[string]bool{
	"show":     true,
	"status":   true,
	"log":      true,
	"diff":     true,
	"pull":     true,
	"push":     true,
	"add":      true,
	"commit":   true,
	"checkout": true,
	"branch":   true,
	"reset":    true,
	"merge":    true,
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
        pre { background: #f4f4f4; padding: 10px; white-space: pre-wrap; }
    </style>
</head>
<body>
    <h2>Git Status</h2>
    <pre>%s</pre>
    <h2>Create PR with Edits</h2>
    <button onclick="createPR()">Create PR</button>
    <pre id="prOutput"></pre>
    <h2>Pull Origin Main</h2>
    <button id="pullOriginMainButton">Pull origin/main</button>
    <h3>Pull Output:</h3>
    <pre id="pullOutput"></pre>

    <h2>Run Git Command</h2>
    <form id="gitForm">
      <input type="text" id="command" placeholder="Enter git command (e.g., log --oneline)" style="width: 80%%;">
      <button type="submit">Run</button>
    </form>
    <h3>Output:</h3>
    <pre id="output"></pre>

    <script>
    // Fill the command textbox with the pull command and submit
    document.getElementById("pullOriginMainButton").onclick = function() {
      // Fill in the command input with "git pull origin main"
      document.getElementById("command").value = "pull origin main";
      
      // Automatically submit the form
      document.getElementById("gitForm").submit();
    };

    function createPR() {
        fetch("/git/create-pr-with-edits", { method: "POST" })
            .then(resp => resp.text())
            .then(text => {
                document.getElementById("prOutput").innerText = text;
            })
            .catch(err => {
                document.getElementById("prOutput").innerText = "Error: " + err;
            });
    }
    document.getElementById("gitForm").onsubmit = async function(event) {
            event.preventDefault();
            let commandStr = document.getElementById("command").value.trim();

            if (commandStr.length === 0) {
                alert("Please enter a command.");
                return;
            }

            let commandParts = commandStr.split(" ");
            let response = await fetch("/git/run", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ command: commandParts })
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
		log.Println(err)
		return
	}

	// Extract base command
	baseCmd := cmd.Command[0]

	// Check if command is allowed
	if !allowedCommands[baseCmd] {
		http.Error(w, "Forbidden command", http.StatusForbidden)
		return
	}

	// Special case for `git commit -m "message"`
	if baseCmd == "commit" {
		if len(cmd.Command) < 3 || cmd.Command[1] != "-m" {
			http.Error(w, "Invalid commit format. Use: commit -m \"message\"", http.StatusBadRequest)
			return
		}
		// Rejoin commit message
		msg := strings.Join(cmd.Command[2:], " ")
		output, err := runGit("commit", "-m", msg)
		if err != nil {
			http.Error(w, output, http.StatusInternalServerError)
			log.Println(err)
			return
		}
		w.Write([]byte(output))
		return
	}

	// Run generic allowed commands
	output, err := runGit(cmd.Command...)
	if err != nil {
		http.Error(w, output, http.StatusInternalServerError)
		log.Println(err)
		return
	}

	w.Write([]byte(output))
}

// createPRHandler: Creates a PR after committing main.bean to edit branch
func createPrHandler(w http.ResponseWriter, r *http.Request) {
	// Step 1: Get current branch
	currentBranch, err := runGit("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		http.Error(w, "Failed to get current branch: "+err.Error(), http.StatusInternalServerError)
		return
	}
	currentBranch = strings.TrimSpace(currentBranch)

	// Defer switch back to main
	defer func() {
		_, err := runGit("checkout", "main")
		if err != nil {
			log.Printf("Failed to switch back to main branch: %v", err)
		}
		_, err = runGit("pull", "origin", "main")
		if err != nil {
			log.Printf("Failed to pull origin main: %v", err)
		}
	}()

	// Step 2: Switch to "edit" branch if not already on it
	if currentBranch != "edit" {
		_, err := runGit("checkout", "-B", "edit")
		if err != nil {
			http.Error(w, "Failed to switch to edit branch: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Step 3: Add main.bean
	_, err = runGit("add", "main.bean")
	if err != nil {
		http.Error(w, "Failed to add file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Step 4: Check for staged changes
	diffCmd := exec.Command("git", "diff", "--cached", "--quiet")
	diffCmd.Dir = gitRepoPath
	err = diffCmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// There are staged changes
			_, err = runGit("commit", "-m", "add data")
			if err != nil {
				http.Error(w, "Commit failed: "+err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			http.Error(w, "Error checking staged changes: "+err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		fmt.Fprintf(w, "No changes to commit")
		log.Println("No changes to commit.")
		return
	}

	// Check if origin/edit exists
	_, err = runGit("ls-remote", "--exit-code", "--heads", "origin", "edit")
	if err == nil {
		// origin/edit exists, merge it too
		_, err = runGit("merge", "origin/edit")
		if err != nil {
			http.Error(w, "Failed to merge origin/edit: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Step 5: Push the branch to origin
	_, err = runGit("push", "-u", "origin", "edit")
	if err != nil {
		http.Error(w, "Failed to push branch: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Step 6: Create PR
	cmd := exec.Command("gh", "pr", "create", "--fill")
	cmd.Dir = gitRepoPath

	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		http.Error(w, "Failed to create PR: "+stderr.String()+"\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	// Step 7: Return PR output
	w.Write([]byte(out.String()))
}

// main starts the server
func main() {
	// Parse optional port argument
	port := flag.String("port", "7001", "Port to run the server on")
	flag.Parse()

	// Ensure the Git repo directory exists
	absPath, err := filepath.Abs(gitRepoPath)
	if err != nil {
		log.Fatalf("Invalid repo path: %v", err)
	}
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		log.Fatalf("Git repo directory does not exist: %s", absPath)
	}

	addr := "127.0.0.1:" + *port
	log.Println("Git server running on", addr, "in directory:", absPath)
	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/git/run", gitCommandHandler)
	http.HandleFunc("/git/create-pr-with-edits", createPrHandler)
	log.Fatal(http.ListenAndServe(addr, nil))
}
