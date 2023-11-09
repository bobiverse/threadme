package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"github.com/fatih/semgroup"
	"golang.org/x/net/context"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
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
	// whileMsg := flag.String("while", "", "Keep running while all have this message")
	flag.Parse()

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

	var flines []string
	if _, err := os.Stat(*fpath); err == nil {
		lines, err := readLines(*fpath)
		if err != nil {
			log.Fatalf("\n\nFILE READ ERROR!\n%s\n", err)
			return
		}
		flines = lines
	}

	fmt.Printf("%20s: [%s]\n", "Command", *cmd)
	if len(flines) > 0 {
		fmt.Printf("%20s: [%s]\n", "File to read", *fpath)
		fmt.Printf("%20s: [%d]\n", "Lines in file", len(flines))
	} else {
		fmt.Printf("%20s: [%d]\n", "Job count to perform", *n)
	}
	fmt.Printf("%20s: [%d]\n", "Threads", *threads)
	fmt.Printf("%20s: %d ms\n", "Delay", *delayMs)
	fmt.Printf("%20s: %d ms\n", "Timeout for single job", *wtime)

	if *stopOnMsg != "" {
		fmt.Printf("%20s: '%s'\n", "Stop if contains", *stopOnMsg)
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
		log.Printf("> Duration: %s ", time.Since(tStart))
		os.Exit(1)
	}()

	// fill `flines` with `n` numbers and use same `for` loop
	if len(flines) == 0 {
		for i := 0; i < *n; i++ {
			flines = append(flines, strconv.Itoa(i))
		}
	}

	// lesgoooooo!
	total := len(flines)
	for i, fline := range flines {
		i := i
		flcmd := *cmd
		flcmd = strings.ReplaceAll(flcmd, "{{N}}", fmt.Sprintf("%d", i))
		flcmd = strings.ReplaceAll(flcmd, "{{LINE}}", fline)

		sg.Go(func() error {
			// log.Printf("`%v`", flcmd)
			cmdOut, errBuf, err := runBashWithTimeout(time.Duration(*wtime)*time.Millisecond, flcmd)
			cmdOut = bytes.TrimSpace(cmdOut)
			// log.Printf("%v -- %v -- %v", cmdOut, errBuf, err)

			errStr := ""
			if errBuf != nil && len(errBuf) > 0 {
				errStr += strings.TrimSpace(string(errBuf)) + "; "
			}

			if err != nil {
				errStr += strings.TrimSpace(string(err.Error())) + "; "
			}

			errStr = strings.TrimSpace(errStr)
			if errStr != "" {
				log.Printf("ERROR: [%d/%d] [%s] ==> [%s]", i, total, flcmd, errStr)
				// do not mark semgroup job with error. we printed it
				return nil

			}
			log.Printf("[%d/%d] [%s] ==> [%s]", i, total, flcmd, cmdOut)

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
	name := "/bin/sh"
	args := []string{
		"-c",
		strings.TrimPrefix(cmdstr, "/bin/sh -c "), // as one argument
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

	if timeout > 0 {
		go func() {
			time.Sleep(timeout) // wait in background

			pgid, err := syscall.Getpgid(cmd.Process.Pid)
			if err == nil {
				// log.Printf("[ KILL ] Kill process of command: %s", name)
				if err := syscall.Kill(-pgid, 15); err != nil { // note the minus sign
					// skip error check
					log.Printf("(Warning: %s)", err)
				}
			}

		}()
	}

	err := cmd.Wait()
	return bufOut.Bytes(), bufErr.Bytes(), err
}
