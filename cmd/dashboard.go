package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"

	"github.com/kubeshop/testkube/pkg/process"
	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/kubeshop/kusk/k8s"
)

var (
	kubeConfig                      string
	dashboardEnvoyFleetName         string
	dashboardEnvoyFleetNamespace    string
	dashboardEnvoyFleetExternalPort int
)

func init() {
	rootCmd.AddCommand(dashboardCmd)

	kubeConfigDefault := ""
	if home := homeDir(); home != "" {
		kubeConfigDefault = filepath.Join(home, ".kube", "config")
	}

	dashboardCmd.Flags().StringVarP(&kubeConfig, "kubeconfig", "", kubeConfigDefault, "absolute path to kube config")
	dashboardCmd.Flags().StringVarP(&dashboardEnvoyFleetNamespace, "envoyfleet.namespace", "", "kusk-system", "kusk gateway dashboard namespace")
	dashboardCmd.Flags().StringVarP(&dashboardEnvoyFleetName, "envoyfleet.name", "", "kusk-gateway-private-envoy-fleet", "kusk gateway dashboard service name")
	dashboardCmd.Flags().IntVarP(&dashboardEnvoyFleetExternalPort, "external-port", "", 8080, "external port to access dashboard at")
}

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Access the kusk dashboard",
	Run: func(cmd *cobra.Command, args []string) {

		// use the current context in kubeconfig
		config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		podList, err := clientset.CoreV1().Pods(dashboardEnvoyFleetNamespace).List(context.Background(), metav1.ListOptions{
			LabelSelector: fmt.Sprintf("fleet=%s.%s", dashboardEnvoyFleetName, dashboardEnvoyFleetNamespace),
		})
		if err != nil || len(podList.Items) == 0 {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		var chosenPod v1.Pod
		for _, pod := range podList.Items {
			// pick the first pod found to be running
			if pod.Status.Phase == v1.PodRunning {
				chosenPod = pod
			}
		}

		// stopCh controls the port forwarding lifecycle.
		// When it gets closed the port forward will terminate
		stopCh := make(chan struct{}, 1)
		// readyCh communicates when the port forward is ready to receive traffic
		readyCh := make(chan struct{})

		// managing termination signal from the terminal.
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			<-sigs
			fmt.Println("Exiting...")
			close(stopCh)
			wg.Done()
		}()

		go func() {
			err := k8s.PortForward(k8s.PortForwardRequest{
				RestConfig: config,
				Pod: v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      chosenPod.Name,
						Namespace: chosenPod.Namespace,
					},
				},
				ExternalPort: dashboardEnvoyFleetExternalPort,
				InternalPort: 8080,
				StopCh:       stopCh,
				ReadyCh:      readyCh,
			})
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}()

		<-readyCh

		browserOpenCMD, browserOpenArgs := getBrowserOpenCmdAndArgs("http://localhost:8080")
		process.Execute(browserOpenCMD, browserOpenArgs...)
		wg.Wait()
	},
}

// open opens the specified URL in the default browser of the user.
func getBrowserOpenCmdAndArgs(url string) (string, []string) {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)

	return cmd, args
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
