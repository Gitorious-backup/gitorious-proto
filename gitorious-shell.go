package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

func getenv(name, defaultValue string) string {
	value := os.Getenv(name)

	if value == "" {
		value = defaultValue
	}

	return value
}

func configureLogger(logfilePath, clientId string) func() {
	f, err := os.OpenFile(logfilePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}

	log.SetOutput(f)
	log.SetPrefix(fmt.Sprintf("[%v] ", clientId))

	return func() { f.Close() }
}

var gitCommandRegexp = regexp.MustCompile("^(git(-|\\s)(receive-pack|upload-pack|upload-archive))\\s+'([^']+)'$")

func parseGitCommand(fullCommand string) (string, string, error) {
	matches := gitCommandRegexp.FindStringSubmatch(fullCommand)
	if matches == nil {
		return "", "", errors.New(fmt.Sprintf("invalid git-shell command \"%v\"", fullCommand))
	}

	return matches[1], matches[4], nil
}

func getRealRepoPath(repoPath string) (string, error) {
	return "real/path.git", nil
}

func getFullRepoPath(repoPath, reposRootPath string) (string, error) {
	fullRepoPath := filepath.Join(reposRootPath, repoPath)

	preReceiveHookPath := filepath.Join(fullRepoPath, "hooks", "pre-receive")
	if info, err := os.Stat(preReceiveHookPath); err != nil || info.Mode()&0111 == 0 {
		return "", errors.New("pre-receive hook is missing or is not executable")
	}

	return fullRepoPath, nil
}

func formatGitShellCommand(command, repoPath string) string {
	return fmt.Sprintf("%v '%v'", command, repoPath)
}

func execGitShell(command string) (string, error) {
	var stderrBuf bytes.Buffer
	cmd := exec.Command("git-shell", "-c", command)
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		return strings.Trim(stderrBuf.String(), " \n"), err
	}

	return "", nil
}

func main() {
	clientId := getenv("SSH_CLIENT", "local")
	logfilePath := getenv("LOGFILE", "/tmp/gitorious-shell.log")
	reposRootPath := getenv("REPOSITORIES", "/var/www/gitorious/repositories")

	closeLogger := configureLogger(logfilePath, clientId)
	defer closeLogger()

	log.Printf("client connected")

	if len(os.Args) < 2 {
		fmt.Println("Error occured, please contact support")
		log.Fatalf("username argument missing, check .authorized_keys file")
	}

	username := os.Args[1]

	ssh_original_command := strings.Trim(os.Getenv("SSH_ORIGINAL_COMMAND"), " \n")
	if ssh_original_command == "" { // deny regular ssh login attempts
		fmt.Printf("Hey %v! Sorry, Gitorious doesn't provide shell access. Bye!\n", username)
		log.Fatalf("SSH_ORIGINAL_COMMAND missing, aborting...")
	}

	command, repoPath, err := parseGitCommand(ssh_original_command)
	if err != nil {
		fmt.Println("Invalid git-shell command")
		log.Fatalf("%v, aborting...", err)
	}

	realRepoPath, err := getRealRepoPath(repoPath)
	if err != nil {
		fmt.Println("Access denied or invalid repository path")
		log.Fatalf("%v, aborting...", err)
	}

	fullRepoPath, err := getFullRepoPath(realRepoPath, reposRootPath)
	if err != nil {
		fmt.Println("Fatal error, please contact support")
		log.Fatalf("%v, aborting...", err)
	}

	gitShellCommand := formatGitShellCommand(command, fullRepoPath)
	log.Printf("invoking git-shell with \"%v\"", gitShellCommand)

	syscall.Umask(0022) // set umask for pushes

	if stderr, err := execGitShell(gitShellCommand); err != nil {
		fmt.Println("Fatal error, please contact support")
		log.Printf("error occured in git-shell: %v", err)
		log.Fatalf("stderr: %v", stderr)
	}

	log.Printf("client disconnected, all ok")
}
