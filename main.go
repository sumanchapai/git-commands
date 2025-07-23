package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Get Git repository path from ENV variable, fallback if not set
var GitRepoPath = getRepoPath()
var HBLReportsDir = filepath.Join(GitRepoPath, "hbl-swipe-statements/reports")

func getRepoURL() string {
	if path, exists := os.LookupEnv("GIT_REPO_URL"); exists {
		return path
	}
	return "https://github.com/sumanchapai/superview-accounting"
}

func getRepoPath() string {
	if path, exists := os.LookupEnv("GIT_REPO_PATH"); exists {
		return path
	}
	return "/Users/suman/Desktop/projects/superview/superview-accounting"
}

func makeQueriesString() string {
	var v strings.Builder
	for _, q := range beancountQueries {
		v.WriteString(fmt.Sprintf(`<div>
      <pre style='border: 1px solid black; padding: 8px 4px'>%v</pre></div>`, q.query))
	}
	return v.String()
}

// GitCommand represents a request to run a git command.
type GitCommand struct {
	Command []string `json:"command"`
}

// runGit executes a git command inside the Git repo directory.
func runGit(command ...string) (string, error) {
	cmd := exec.Command("git", command...)
	cmd.Dir = GitRepoPath // Enforce the working directory

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

var beancountQueries = []struct {
	query string
}{
	{`
SELECT
    YEAR(date) AS year,
    MONTH(date) AS month,
    account,
    SUM(position) AS total
WHERE
    account ~ "Income:Room" OR account ~ "Income:Restaurant"
GROUP BY
    year, month, account
ORDER BY
    year ASC, month ASC, account ASC
      `},
}

// rootHandler: Serve the main page with Git status and a command input form
func rootHandler(w http.ResponseWriter, r *http.Request) {
	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Git Server</title>
    <style>
      body { font-family: Arial, sans-serif; margin: 20px; }
      textarea { width: 100%%; height: 100px; }
      pre { background: #f4f4f4; padding: 10px; white-space: pre-wrap; font-family: monospace; }

      .diff-output {
      background: #f6f8fa;
      border: 1px solid #ccc;
      padding: 10px;
      overflow-x: auto;
      white-space: pre;
      overflow-y: auto;
      max-height: 60vh;
      }

      #output {
      border: 1px solid #ccc;
      padding: 10px;
      overflow-x: auto;
      white-space: pre;
      overflow-y: auto;
      max-height: 40vh;
      }

      .diff-addition { color: #22863a; background-color: #e6ffed; }
      .diff-deletion { color: #b31d28; background-color: #ffeef0; }
      .diff-hunk     { color: #6a737d; font-weight: bold; }

      .responsive-grid {
        display: grid;
        grid-template-columns: 1fr; /* default: single column on small screens */
        gap: 1rem;
      }

    @media (min-width: 768px) {
      .responsive-grid {
        grid-template-columns: 40%% 60%%; /* two columns on medium and up */
      }
    }
    </style>
</head>
<body>
  <a href="%s">%s</a>
  <div class="responsive-grid">
    <div>
      <div style="border: 1px solid black; margin-top: 2rem;">
      <h2>Create PR with Edits</h2>
      <button onclick="createPR()">Create PR</button>
      <pre id="prOutput"></pre>
      </div>

      <div style="border: 1px solid orange; margin-top: 2rem;">
      <h2>Run Git Command</h2>
      <form id="gitForm">
        <input type="text" id="command" placeholder="Enter git command (e.g., log --oneline)" style="width: 80%%;">
        <button type="submit">Run</button>
      </form>
      <h3>Output:</h3>
      <pre id="output"></pre>
      </div>

      <div style="border: 1px solid purple; margin-top: 2rem;">
      <h2>Beancount Query</h2>
      <p>To ignore positive income that is intented, mention "intended positive" in narration.</p>
      <button onclick="positiveIncome()">Positive Incomes</button>
      <button onclick="bankCharge()">Missing bank charges for HBL Income</button>
      <form id="beanQueryForm" style="margin-top: 1rem">
        <input type="text" id="bean-query-command" placeholder="Enter beancount query" style="width: 80%%;">
        <button type="submit">Run</button>
      </form>

      <h3>Output:</h3>
      <pre id="beancount-output"></pre>
      </div>
      <div>
        <p>Other helpful queries:</p>
        %v
      </div>

      <div style="border: 1px solid orange; margin-top: 2rem;">
      <h2>HBL Swipe Statements</h2>
      <a href="/git/hbl">View Reports</a>
      <div style="margin-top: 1rem">
        <form id="hblReportQueryForm"/>
        <input id="hbl-report-date" type="date" required />
        <input type="submit" value="Fetch"/>
      </div>
      <div style="margin-top: 1rem"><button onclick="fetchLatestSwipeStatements()">Fetch All Latest</button></div>
      <pre id="hblfetchresult"></pre>
      </div>

    </div>


    <div style="border: 1px solid blue; margin-top: 2rem">
    <h2 >Git Diff</h2>
    <h3>Output:</h3>
    <pre id="diffOutput" class="diff-output">Loading git diff...</pre>

    </div>
  </div>

    <script>

    function positiveIncome() {
      const input = document.getElementById("bean-query-command")
      input.value = 'select date, lineno, account, narration, position where account ~ "Income" and narration !~ "refund" and narration !~ "intended positive" and number > 0'
      // Submit the bean-query form
      document.getElementById("beanQueryForm").requestSubmit()
    }

    function bankCharge() {
      const input = document.getElementById("bean-query-command")
      input.value = 'select date, lineno, account, position, narration where has_account("Assets:Bank:HBL") and has_account("Income") and account = "Assets:Bank:HBL" and not has_account("Expenses:BankCharge") and flag = "*"'

      // Submit the bean-query form
      document.getElementById("beanQueryForm").requestSubmit()
    }

    document.getElementById("beanQueryForm").onsubmit = function(event) {
      event.preventDefault()
      const commandStr = document.getElementById("bean-query-command").value.trim()

      if (commandStr.length === 0) {
          alert("Please enter a command.");
          return;
      }

      fetch("/git/bean-query", {
         method: "POST",
         headers: { "Content-Type": "text/plain" },
         body: commandStr
      }).then(x => x.text()).then(x => {
        document.getElementById("beancount-output").innerText = x;
      }).catch(err => {
          document.getElementById("beancount-output").innerText = "Error: " + err;
      });
    }

    function createPR() {
        let message = prompt("Enter your commit message:")?.trim();
        if (!message) {
            alert("Commit cancelled.");
            return;
        }

        const prOutput = document.getElementById("prOutput");
        prOutput.innerText = "Waiting for server response...";

        fetch("/git/create-pr-with-edits?commit_msg=" + encodeURIComponent(message), { method: "POST" })
            .then(resp => resp.text())
            .then(text => {
                text = text.trim();
                const words = text.split(/\s+/);

                if (words.length === 1 && (text.startsWith("http://") || text.startsWith("https://"))) {
                    // Single link
                    prOutput.innerHTML = '<a href="' + text + '" target="_blank">' + text + '</a>';
                } else {
                    // Regular text or multi-line output
                    prOutput.innerText = text;
                }
            })
            .catch(err => {
                prOutput.innerText = "Error: " + err;
            }).finally(refreshDiff);
    }

    document.getElementById("gitForm").onsubmit = function(event) {
            event.preventDefault();
            let commandStr = document.getElementById("command").value.trim();

            if (commandStr.length === 0) {
                alert("Please enter a command.");
                return;
            }

            let commandParts = commandStr.split(" ");
            document.getElementById("output").innerText = "Waiting for server response...";
            fetch("/git/run", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ command: commandParts })
            }).then(x => x.text()).then(x => {
              document.getElementById("output").innerText = x;
            }).catch(err => {
                document.getElementById("output").innerText = "Error: " + err;
            }).finally(refreshDiff);
        };


     // This formats the git diff output to add colors like GitHub
    function formatGitDiff(diffText) {
        const lines = diffText.split('\n');
        return lines.map(line => {
            if (line.startsWith('+') && !line.startsWith('+++')) {
                return '<span class="diff-addition">' + escapeHtml(line) + '</span>';
            } else if (line.startsWith('-') && !line.startsWith('---')) {
                return '<span class="diff-deletion">' + escapeHtml(line) + '</span>';
            } else if (line.startsWith('@@')) {
                return '<span class="diff-hunk">' + escapeHtml(line) + '</span>';
            } else {
                return escapeHtml(line);
            }
        }).join('\n');
    }

    function escapeHtml(text) {
        return text
            .replace(/&/g, "&amp;")
            .replace(/</g, "&lt;")
            .replace(/>/g, "&gt;");
    }

    async function refreshDiff() {
      const resp = await fetch("/git/diff");
      const rawDiff = await resp.text();
      document.getElementById("diffOutput").innerHTML = formatGitDiff(rawDiff);
    }

    function fetchLatestSwipeStatements() {
        document.getElementById("hblfetchresult").innerText = "Loading...";
        fetch("/git/fetch-latest-hbl")
          .then(x => x.text()).then(x => {
          document.getElementById("hblfetchresult").innerText = x;
          }).catch(err => {
              document.getElementById("hblfetchresult").innerText = "Error: " + err;
          });
    }

    document.getElementById("hblReportQueryForm").onsubmit = function(event) {
      event.preventDefault()
      const date = document.getElementById("hbl-report-date").value
      document.getElementById("hblfetchresult").innerText = "Loading...";
      fetch("/git/fetch-hbl-report/?date=" + date)
        .then(x => x.text()).then(x => {
        document.getElementById("hblfetchresult").innerText = x;
        }).catch(err => {
            document.getElementById("hblfetchresult").innerText = "Error: " + err;
        });
    }

    window.onload = refreshDiff;

    </script>
</body>
</html>`, getRepoURL(), getRepoURL(), makeQueriesString())

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func diffHandler(w http.ResponseWriter, r *http.Request) {
	gitDiff, err := runGit("diff")
	if err != nil {
		http.Error(w, "Failed to get git diff: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Send plain diff text (you could also return JSON if needed)
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(gitDiff))
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

	// Step 2: Switch to "edit" branch if not already on it
	// Merge origin/edit if it exists
	if currentBranch != "edit" {
		_, err := runGit("checkout", "-B", "edit")
		if err != nil {
			http.Error(w, "Failed to switch to edit branch: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Fetch from origin
	_, err = runGit("fetch", "origin")
	if err != nil {
		http.Error(w, "Failed to fetch origin: "+err.Error(), http.StatusInternalServerError)
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

	// Merge origin/main if exists
	_, err = runGit("ls-remote", "--exit-code", "--heads", "origin", "main")
	if err == nil {
		// origin/main exists, merge it too
		_, err = runGit("merge", "origin/main")
		if err != nil {
			http.Error(w, "Failed to merge origin/main: "+err.Error(), http.StatusInternalServerError)
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
	diffCmd.Dir = GitRepoPath
	err = diffCmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// There are staged changes
			commitMsg := r.URL.Query().Get("commit_msg")
			if commitMsg == "" {
				commitMsg = "Add data"
			}
			if len(commitMsg) > 300 {
				commitMsg = commitMsg[:300] + "…"
			}
			authorEmail := r.Header.Get("Cf-Access-Authenticated-User-Email")
			if authorEmail != "" {
				_, err = runGit("commit", "-m", commitMsg, "--author", fmt.Sprintf("X <%s>", authorEmail))
			} else {
				_, err = runGit("commit", "-m", commitMsg)
			}
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
		return
	}

	// Step 5: Push the branch to origin
	_, err = runGit("push", "-u", "origin", "edit")
	if err != nil {
		http.Error(w, "Failed to push branch: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Step 6: Check if an open PR already exists for 'edit' branch
	checkPRCmd := exec.Command("gh", "pr", "list", "--head", "edit", "--state", "open")
	checkPRCmd.Dir = GitRepoPath
	var out bytes.Buffer
	checkPRCmd.Stdout = &out
	checkPRCmd.Stderr = &out

	if err := checkPRCmd.Run(); err != nil {
		w.Write([]byte("Error checking for existing PR:\n" + err.Error() + out.String()))
		return
	}

	if strings.TrimSpace(out.String()) != "" {
		// An open PR already exists for 'edit'
		checkPRCmd = exec.Command("gh", "pr", "view", "edit", "--json", "url", "-t", "{{.url}}\n")
		checkPRCmd.Dir = GitRepoPath
		out = bytes.Buffer{}
		checkPRCmd.Stdout = &out
		checkPRCmd.Stderr = &out
		if err := checkPRCmd.Run(); err != nil {
			w.Write([]byte("Error listing existing PR:\n" + err.Error() + out.String()))
			return
		}
		w.Write([]byte(out.String()))
		return
	}

	// Step 7: Create PR since none exists
	cmd := exec.Command("gh", "pr", "create", "--fill")
	cmd.Dir = GitRepoPath

	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		http.Error(w, "Failed to create PR: "+stderr.String()+"\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	// Step 8: Return PR output
	w.Write([]byte(out.String()))
}

func beanQueryHandler(w http.ResponseWriter, r *http.Request) {
	// Get the query string
	queryString, err := io.ReadAll(r.Body)
	if err != nil {
		errMsg := fmt.Sprintf("Error reading query string %v", err.Error())
		http.Error(w, errMsg, http.StatusInternalServerError)
	}
	// Execute the command
	cmd := exec.Command("bean-query", "main.bean", string(queryString))
	cmd.Dir = GitRepoPath
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		http.Error(w, "Failed to create PR: "+stderr.String()+"\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	// Step 8: Return PR output
	w.Write([]byte(out.String()))
}

// Get the date for which there exists HBL swipe statement in the HBLReportsDir
// If no date exists, get some arbitrary default date
func lastReportDate() (string, error) {
	re := regexp.MustCompile(`^report-(\d{4}-\d{2}-\d{2})\.(?:pdf|no-data)$`)
	latest := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	err := filepath.WalkDir(HBLReportsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		filename := d.Name()
		matches := re.FindStringSubmatch(filename)
		if len(matches) == 2 {
			dateStr := matches[1]
			date, err := time.Parse("2006-01-02", dateStr)
			if err != nil {
				return err
			}
			if date.After(latest) {
				latest = date
			}
		}
		return nil
	})

	if err != nil {
		return "", err
	}
	if latest.IsZero() {
		return "", fmt.Errorf("no matching report files found")
	}
	return latest.Format("2006-01-02"), nil
}

func fetchLatestHBLSwipesHandler(w http.ResponseWriter, r *http.Request) {
	dateFormat := "2006-01-02"
	fromDate, err := lastReportDate()
	if err != nil {
		http.Error(w, "Failed to get last report date: "+"\n"+err.Error(), http.StatusInternalServerError)
		return
	}
	todayDate := time.Now().Format(dateFormat)
	// Execute the command
	cmd := exec.Command("go", "run", "download.go", fromDate, todayDate)
	cmd.Dir = filepath.Join(HBLReportsDir, "..")
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		http.Error(w, "Failed to download HBL reports: "+stderr.String()+"\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	// Step 8: Return PR output
	w.Write([]byte(out.String()))
}

func fetchHBLReportHandler(w http.ResponseWriter, r *http.Request) {
	dateFormat := "2006-01-02"
	fromDate := r.URL.Query().Get("date")
	_, err := time.Parse(dateFormat, fromDate)
	if err != nil {
		http.Error(w, "Invalid date string: "+"\n"+err.Error(), http.StatusInternalServerError)
		return
	}
	// Execute the command
	cmd := exec.Command("go", "run", "download.go", fromDate, fromDate)
	cmd.Dir = filepath.Join(HBLReportsDir, "..")
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		http.Error(w, "Failed to download HBL reports: "+stderr.String()+"\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	// Step 8: Return PR output
	w.Write([]byte(out.String()))
}

// main starts the server
func main() {
	// Parse optional port argument
	port := flag.String("port", "7001", "Port to run the server on")
	flag.Parse()

	// Ensure the Git repo directory exists
	absPath, err := filepath.Abs(GitRepoPath)
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
	http.HandleFunc("/git/diff", diffHandler)
	http.HandleFunc("/git/bean-query", beanQueryHandler)

	fs := http.FileServer(http.Dir(HBLReportsDir))
	http.Handle("/git/hbl/", http.StripPrefix("/git/hbl/", fs))
	// TODO:
	// Global rate limit this API to prevent overwhelming HBL server.
	// 10 requests per day max
	http.HandleFunc("/git/fetch-latest-hbl/", fetchLatestHBLSwipesHandler)
	http.HandleFunc("/git/fetch-hbl-report/", fetchHBLReportHandler)

	log.Fatal(http.ListenAndServe(addr, nil))
}
