package main

import (
	"github.com/pborman/getopt"
	"fmt"
	"log"
	"os"
	"time"
)

// global variables
var Template string
var ErrTemplate string

// commandline arguments
var fCommand string
var fUser *string
var fDelay *int
var fProcs *int
var fFile *string
var fNoStrict *bool
var fHelp *bool

// initialize
func init() {
	// commandline arguments
	fUser = getopt.StringLong("user", 'u', "root", "ssh login as this username")
	fFile = getopt.StringLong("file", 'f', "", "file with the list of hosts")
	fDelay = getopt.IntLong("delay", 'd', 100, "delay between each ssh fork (default 100 msec)")
	fProcs = getopt.IntLong("procs", 'p', 500, "number of parallel ssh processes (default: 500)")
	fNoStrict = getopt.BoolLong("nostrict", 'n', "don't use strict ssh fingerprint checking")
	fHelp = getopt.BoolLong("help", 'h', "show this help screen")

	// initialize output template strings
	Template = "%*s%s \033[01;32m->\033[0m %s"
	ErrTemplate = "%*s%s \033[01;31m=>\033[0m %s"

	// disable colored output in case output is redirected
	if !IsTerminal(os.Stdout.Fd()) {
		Template = "%*s%s -> %s"
	}
	if !IsTerminal(os.Stderr.Fd()) {
		ErrTemplate = "%*s%s => %s"
	}
}

// main program
func main() {
	// local variables
	var err error

	// parse commandline argiments
	getopt.Parse()

	// show help screen and exit in case of -h or --help option
	if *fHelp {
		getopt.Usage()
		os.Exit(1)
	}

	// look for mandatory positional arguments
	if getopt.NArgs() < 1 {
		log.Fatal("Nothing to do. Use -h for help.")
	}

	// by default, read server list from stdin
	ServerListFile := os.Stdin

	// read server names from file if a file name is supplied
	if *fFile != "" {
		ServerListFile, err = os.Open(*fFile)
		if err != nil {
			log.Fatal(fmt.Sprintf("ServerListFile: Error: %v", err))
		}
		defer ServerListFile.Close()
	}
	AddrPadding, ServerList := LoadServerList(ServerListFile)

	// command to run on servers
	fCommand = getopt.Arg(0)

	// make new group
	group := &SshGroup{
		Active:   0,
		Total:    len(ServerList),
		Complete: 0,
	}

	// no point to display more processes than
	if *fProcs > group.Total {
		*fProcs = group.Total
	}

	// print heading text
	fmt.Fprintln(os.Stderr, "gssh - group ssh, ver. 0.6")
	fmt.Fprintln(os.Stderr, "(c)2014 Bozhin Zafirov <bozhin@deck17.com>")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "  [*] read (%d) hosts from the list\n", group.Total)
	fmt.Fprintf(os.Stderr, "  [*] executing '%s' as user '%s'\n", fCommand, *fUser)
	fmt.Fprintf(os.Stderr, "  [*] spawning %d parallel ssh sessions\n\n", *fProcs)

	// spawn ssh processes
	for i, Server := range ServerList {
		ssh := &SshServer{
			Username: *fUser,
			Address:  Server,
		}
		group.Servers = append(group.Servers, ssh)
		// run command
		group.mu.Lock()
		group.Active++
		group.mu.Unlock()
		go group.Command(ssh, AddrPadding, fCommand)
		// show progless after new process spawn
		group.UpdateProgress()
		if i < group.Total {
			// time delay and max procs wait between spawn
			time.Sleep(time.Duration(*fDelay) * time.Millisecond)
			group.Wait(*fProcs)
		}
	}
	// wait for ssh processes to exit
	group.Wait(0)
	group.mu.Lock()
	group.ClearProgress()
	group.mu.Unlock()

	// calculate stats
	var StdoutServersCount int
	var StderrServersCount int
	var AllServersCount int
	var StdoutLinesCount int
	var StderrLinesCount int
	var AllLinesCount int
	for _, ssh := range group.Servers {
		if ssh.StdoutLineCount > 0 {
			StdoutLinesCount += ssh.StdoutLineCount
			StdoutServersCount++
		}
		if ssh.StderrLineCount > 0 {
			StderrLinesCount += ssh.StderrLineCount
			StderrServersCount++
		}
		if ssh.StdoutLineCount > 0 || ssh.StderrLineCount > 0 {
			AllLinesCount += ssh.StdoutLineCount + ssh.StderrLineCount
			AllServersCount++
		}
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "  Done. Processed: %d / Output: %d (%d) / \033[01;32m->\033[0m %d (%d) / \033[01;31m=>\033[0m %d (%d)\n",
		group.Total,
		AllServersCount,
		AllLinesCount,
		StdoutServersCount,
		StdoutLinesCount,
		StderrServersCount,
		StderrLinesCount,
	)
}
