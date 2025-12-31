package samples

import (
	"net/http"
	"os/exec"
)

// PingHandler pings a host - VULNERABLE TO COMMAND INJECTION
func PingHandler(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")

	// BAD: Direct user input passed to shell command
	cmd := exec.Command("sh", "-c", "ping -c 1 "+host)
	output, err := cmd.Output()
	if err != nil {
		http.Error(w, "Ping failed", 500)
		return
	}
	w.Write(output)
}

// LookupHandler performs DNS lookup - VULNERABLE TO COMMAND INJECTION
func LookupHandler(w http.ResponseWriter, r *http.Request) {
	domain := r.FormValue("domain")

	// BAD: User input in command string
	cmd := exec.Command("bash", "-c", "nslookup "+domain)
	output, _ := cmd.CombinedOutput()
	w.Write(output)
}

// FileReader reads a file - VULNERABLE TO PATH TRAVERSAL
func FileReader(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Query().Get("file")

	// BAD: No path validation, allows ../../../etc/passwd
	cmd := exec.Command("cat", filename)
	output, _ := cmd.Output()
	w.Write(output)
}
