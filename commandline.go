// commandline.go contains logic and data structures relevant to actually
// running peirates as a command line tool. Mainly this is just flag handling.
package peirates

import (
	"flag" // Command line flag parsing
	"log"
	"os"
	"strings"
)

type CommandLineOptions struct {
	connectionConfig      *ServerInfo
	commandToRunInPods    string
	podsToRunTheCommandIn []string
}

// parseOptions parses command-line options. We call it in main().
// func parseOptions(connectionString *ServerInfo, kubeData *Kube_Data) {
func parseOptions(opts *CommandLineOptions) {
	// This is like the parser.add_option stuff except
	// it works implicitly on a global parser instance.
	// Notice the use of pointers (&connectionString.RIPAddress for
	// example) to bind flags to variables

	// After parsing flags, this string will be split on commas and stored
	// as a list in opts.podsToRunTheCommandIn
	var podListRaw string

	flagset := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	flagset.StringVar(&opts.connectionConfig.RIPAddress, "i", opts.connectionConfig.RIPAddress, "API Server IP address: ex. 10.22.34.67")
	flagset.StringVar(&opts.connectionConfig.RPort, "p", opts.connectionConfig.RPort, "API Server Port: ex 443, 6443")
	flagset.StringVar(&podListRaw, "L", "", "List of comma-seperated Pods: ex pod1,pod2,pod3")
	flagset.StringVar(&opts.commandToRunInPods, "c", "hostname", "Command to run in pods")

	// This is the function that actually runs the parser
	// once you've defined all your options.
	flagset.Parse(os.Args[1:])

	// If the IP or Port are an empty string, we want
	// to just print out usage and crash because they have
	// to be defined
	if opts.connectionConfig.RIPAddress == "" {
		// flag.Usage() prints out an auto-generated usage string.
		flagset.Usage()
		// log.Fatal prints a message to stderr and crashes the program.
		log.Fatal("Error: must provide remote IP address (-i)")
	}
	if opts.connectionConfig.RPort == "" {
		// Same as before
		flagset.Usage()
		log.Fatal("Error: must provide remote Port (-p)")
	}

	if podListRaw != "" {
		opts.podsToRunTheCommandIn = strings.Split(podListRaw, ",")
	}
}
