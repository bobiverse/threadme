package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/fatih/semgroup"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var cmdInterpreter = os.Getenv("SHELL")

// Process groups of jobs that are running right now, so cancellation can kill
// them instead of leaving them orphaned when the program exits.
var (
	runningMu sync.Mutex
	running   = map[int]struct{}{}
)

const example = "`./threadme -t5 -cmd 'echo \"{{N}}:{{LINE}}\"'`"

func main() {
	cmd := flag.String("cmd", "", "Command to execute")
	fpath := flag.String("f", "", "file to read for workers and replace {{LINE}} in cmd")
	threads := flag.Int("c", 5, "Concurrency")
	delayMs := flag.Int("d", 10, "Delay in milliseconds")
	wtime := flag.Int("tl", 60*1000, "Time limit for single job in milliseconds")
	n := flag.Int("n", 100, "If no file given this is how many jobs will be performed")
	stopOnMsg := flag.String("stop-on", "", "If any job gets this message, all stops")
	whileMsg := flag.String("while", "", "Keep running while all have this message")
	interpreter := flag.String("interpreter", cmdInterpreter, "Interpreter for command")
	forever := flag.Bool("forever", false, "Run forever")
	flag.Parse()

	// check flag conflicts
	if *forever && *fpath != "" {
		log.Fatalf("You must choose one: `--forever` or `-fpath` flag!")
	}

	shellEnv := cmdInterpreter // package default, read from $SHELL at startup
	if err := checkInterpreter(*interpreter); err != nil {
		log.Fatal(err)
	}
	cmdInterpreter = *interpreter

	*cmd = strings.TrimSpace(*cmd)
	if *cmd == "" {
		log.Fatalf("Empty command. %s", example)
	}

	if *threads <= 1 {
		log.Fatalf("Why you need `threadme` if no concurrency given..? (think)")
	}

	if *delayMs < 0 {
		*delayMs = 1
	}

	if *forever {
		*n = 0
		*delayMs = 250 // larger delay
	}

	var flines []string
	if *fpath != "" {
		_, err := os.Stat(*fpath)
		if err != nil {
			log.Fatalf("\n\nFile `%s` error: %s\n", *fpath, err)
			return
		}

		// file exists, try to read
		lines, err := readLines(*fpath)
		if err != nil {
			log.Fatalf("\n\nFILE READ ERROR!\n%s\n", err)
			return
		}
		flines = lines
	}

	fmt.Printf("%20s: [%s]\n", "Command", *cmd)

	if cmdInterpreter != shellEnv {
		fmt.Printf("%20s: [%s]\n", "Interpreter", cmdInterpreter)
	}

	if len(flines) > 0 {
		fmt.Printf("%20s: [%s]\n", "File to read", *fpath)
		fmt.Printf("%20s: [%d]\n", "Lines in file", len(flines))
	} else if *forever {
		fmt.Printf("%20s: ∞ \n", "Run forever")
	} else {
		fmt.Printf("%20s: [%d]\n", "Job count to perform", *n)
	}
	fmt.Printf("%20s: [%d]\n", "Threads", *threads)
	fmt.Printf("%20s: %d ms\n", "Delay", *delayMs)
	fmt.Printf("%20s: %d ms\n", "Timeout for single job", *wtime)

	if *stopOnMsg != "" {
		fmt.Printf("%20s: '%s'\n", "Stop if contains", *stopOnMsg)
	}

	if *whileMsg != "" {
		fmt.Printf("%20s: '%s'\n", "Continue while", *whileMsg)
	}

	fmt.Println(strings.Repeat("-", 80))

	var tStart = time.Now()
	defer func() {
		log.Printf("> Duration: %s ", time.Since(tStart))
	}()

	var maxWorkers = *threads

	ctx, cancelAll := context.WithCancel(context.Background())
	sg := semgroup.NewGroup(ctx, int64(maxWorkers))

	go func() {
		<-ctx.Done()
		// The context was canceled
		log.Printf("> Stopping all workers!")
		// kill still-running jobs, otherwise os.Exit() orphans them
		killRunning()
		log.Printf("> Duration: %s ", time.Since(tStart))
		os.Exit(1)
	}()

	// fill `flines` with `n` numbers and use same `for` loop
	if len(flines) == 0 {
		for i := 0; i < *n; i++ {
			flines = append(flines, strconv.Itoa(i))
		}
	}
	total := len(flines)
	sTotal := fmt.Sprintf("%d", total)
	if *forever {
		sTotal = "∞"
	}

	for index := 0; ; index++ {
		i := index

		// stop on limit reached if not forever
		if !*forever && (i > total-1) {
			break
		}

		// Replace variables
		flcmd := *cmd
		flcmd = strings.ReplaceAll(flcmd, "{{N}}", fmt.Sprintf("%d", i))

		// Replace variables from file
		if !*forever && i <= total-1 {
			flcmd = strings.ReplaceAll(flcmd, "{{LINE}}", flines[i])
		}

		sg.Go(func() error {
			// log.Printf("`%v`", flcmd)
			cmdOut, errBuf, err := runBashWithTimeout(time.Duration(*wtime)*time.Millisecond, flcmd)
			cmdOut = bytes.TrimSpace(cmdOut)
			// log.Printf("%v -- %v -- %v", cmdOut, errBuf, err)

			errStr := ""
			if len(errBuf) > 0 {
				errStr += strings.TrimSpace(string(errBuf)) + "; "
			}

			if err != nil {
				errStr += strings.TrimSpace(string(err.Error())) + "; "
			}

			errStr = strings.TrimSpace(errStr)
			if errStr != "" {
				log.Printf("ERROR: [%d/%s] [%s] ==> [%s]", i, sTotal, flcmd, errStr)
				// do not mark semgroup job with error. we printed it
				return nil

			}
			log.Printf("[%d/%s] [%s] ==> [%s]", i, sTotal, flcmd, cmdOut)

			// Stop if not expected message
			if *whileMsg != "" && !strings.Contains(string(cmdOut), *whileMsg) {
				log.Printf("> No `while message` found: %s", cmdOut)
				cancelAll()
				return nil
			}

			// Stop all workers when specific string seen
			if *stopOnMsg != "" {
				var needToStop bool
				if strings.Contains(string(cmdOut), *stopOnMsg) {
					log.Printf("> Stop output message found: %s", cmdOut)
					needToStop = true
				}
				if strings.Contains(errStr, *stopOnMsg) {
					log.Printf("> Stop error message found: %s", errStr)
					needToStop = true
				}
				if needToStop {
					cancelAll()
					return nil
				}
			}

			time.Sleep(time.Duration(*delayMs) * time.Millisecond)

			return nil
		})
	}

	if err := sg.Wait(); err != nil {
		fmt.Println(err)
	}
}

// readLines reads a whole file
// and returns a slice of its lines.
func readLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// Execute bash command with valid timeout
// kill process if too long execution
func runBashWithTimeout(timeout time.Duration, cmdstr string) ([]byte, []byte, error) {
	// Run command in env as whole
	// Useful when need to execute command with wildcard so these characters
	// is not treated as string
	name := cmdInterpreter
	args := []string{
		"-c",
		strings.TrimPrefix(cmdstr, cmdInterpreter+" -c "), // as one argument
	}

	cmd := exec.Command(name, args...) // #nosec G204
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	bufOut := &bytes.Buffer{}
	cmd.Stdout = bufOut

	bufErr := &bytes.Buffer{}
	cmd.Stderr = bufErr

	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	// Setpgid made the child a group leader, so its PID is the group ID. Take it
	// now: once cmd.Wait() reaps the child the PID can be recycled, and looking
	// the group up later would point at an unrelated process.
	pgid := cmd.Process.Pid

	addRunning(pgid)
	defer removeRunning(pgid)

	if timeout > 0 {
		timer := time.AfterFunc(timeout, func() {
			killGroup(pgid)
		})
		defer timer.Stop() // job finished in time: no delayed kill
	}

	err := cmd.Wait()
	return bufOut.Bytes(), bufErr.Bytes(), err
}

// killGroup sends SIGTERM to a whole process group, so shell pipelines and
// their grandchildren die too.
func killGroup(pgid int) {
	err := syscall.Kill(-pgid, syscall.SIGTERM) // note the minus sign
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return // gone already: the job finished just before the signal
	}
	log.Printf("(Warning: %s)", err)
}

func addRunning(pgid int) {
	runningMu.Lock()
	running[pgid] = struct{}{}
	runningMu.Unlock()
}

func removeRunning(pgid int) {
	runningMu.Lock()
	delete(running, pgid)
	runningMu.Unlock()
}

// runningPgids returns a snapshot of the process groups running right now.
func runningPgids() []int {
	runningMu.Lock()
	defer runningMu.Unlock()

	pgids := make([]int, 0, len(running))
	for pgid := range running {
		pgids = append(pgids, pgid)
	}
	return pgids
}

// killRunning SIGTERMs every job still running. Best effort: a job that just
// finished is already gone and only logs a warning.
func killRunning() {
	for _, pgid := range runningPgids() {
		killGroup(pgid)
	}
}

// checkInterpreter reports whether jobs have a shell to run in. The
// `--interpreter` flag already defaults to $SHELL, so an empty value here means
// neither was set.
func checkInterpreter(interpreter string) error {
	if interpreter == "" {
		return errors.New("no interpreter: $SHELL is not set and no `--interpreter` flag was given")
	}
	return nil
}
